package main

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"koding/messaging/moh"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"koding/newkite/utils"
	"koding/tools/config"
	"koding/tools/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Storage is an interface that encapsulates basic operations on the kite
// struct. ID is an unique string that belongs to the kite.
type Storage interface {
	// Add inserts the kite into the storage with the kite.ID key. If there
	// is already a kite available with this id, it should update/replace it.
	Add(kite *models.Kite)

	// Get returns the specified kite struct with the given id
	Get(id string) *models.Kite

	// Remove deletes the kite with the given id
	Remove(id string)

	// Has checks whether the kite with the given id exist
	Has(uiid string) bool

	// Size returns the total number of kites in the storage
	Size() int

	// List returns a slice of all kites in the storage
	List() []*models.Kite
}

// Dependency is an interface that encapsulates basic dependency operations
// between a source item and their dependencies. A source's dependency list
// contains no duplicates and all items are unique.
type Dependency interface {
	// Add defines and inserts a new dependency (with the name target) to the
	// source.
	Add(source, target string)

	// Remove deletes source together with all their dependencies. Basically is
	// purges it completely.
	Remove(source string)

	// Has checks whether the dependency tree with the given source name exist.
	Has(source string) bool

	// List returns a slice of all dependencies that depends on the source.
	List(source string) []string
}

type Kontrol struct {
	Replier   *moh.Replier
	Publisher *moh.Publisher
	Port      string
	Hostname  string
}

var (
	storage    Storage
	dependency Dependency
)

func main() {
	hostname, _ := os.Hostname()

	k := &Kontrol{
		Hostname: hostname,
		Port:     strconv.Itoa(config.Current.NewKontrol.Port),
	}

	k.Replier = moh.NewReplier(k.makeRequestHandler())
	k.Publisher = moh.NewPublisher()
	k.Publisher.Authenticate = findUsernameFromSessionID
	k.Publisher.ValidateCommand = validateCommand

	storage = NewMongoDB()
	dependency = NewDependency()

	slog.SetPrefixName("kontrol")
	slog.SetPrefixTimeStamp(time.Stamp)
	slog.Println("started")

	k.Start()
}

func (k *Kontrol) Start() {
	// scheduler functions
	go k.pinger()
	go k.heartBeatChecker()
	rout := mux.NewRouter()
	rout.HandleFunc("/", homeHandler).Methods("GET")
	rout.HandleFunc("/request", prepareHandler(requestHandler)).Methods("POST")
	rout.Handle(moh.DefaultReplierPath, k.Replier)
	rout.Handle(moh.DefaultPublisherPath, k.Publisher)
	http.Handle("/", rout)

	slog.Println(http.ListenAndServe(":"+k.Port, nil))
}

func (k *Kontrol) makeRequestHandler() func([]byte) []byte {
	return func(msg []byte) []byte {
		// slog.Printf("Request came in: %s\n", string(msg))
		result, err := k.handle(msg)
		if err != nil {
			slog.Println(err)
		}

		return result
	}
}

// This is used for two reasons:
// 1. HeartBeat mechanism for kite (Node Coordination)
// 2. Triggering kites to register themself to kontrol (Synchronize PUB/SUB)
func (k *Kontrol) pinger() {
	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	for _ = range ticker.C {
		k.ping()
	}
}

func (k *Kontrol) ping() {
	m := protocol.KontrolMessage{
		Type: protocol.Ping,
	}
	msg, _ := json.Marshal(&m)
	k.Publish("all", msg)
}

// HeartBeat pool checker. Checking for kites if they are live or dead.
// It removes kites from the DB if they are no more alive.
func (k *Kontrol) heartBeatChecker() {
	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	for _ = range ticker.C {
		for _, kite := range storage.List() {
			// Delay is needed to fix network delays, otherwise kites are
			// marked as death even if they are sending 'pongs' to us
			if time.Now().Before(kite.UpdatedAt.Add(protocol.HEARTBEAT_DELAY)) {
				continue // still alive, pick up the next one
			}

			removeLog := fmt.Sprintf("[%s (%s)] dead at '%s' - '%s'",
				kite.Name,
				kite.Version,
				kite.Hostname,
				kite.ID,
			)
			slog.Println(removeLog)

			storage.Remove(kite.ID)

			// only delete from jVMs when all kites to that user is died.
			if !kitesExistsForUser(kite.Username) {
				deleteFromVM(kite.Username)
			}

			stoppedMsg := protocol.KontrolMessage{
				Type: protocol.KiteDisconnected,
				Args: map[string]interface{}{
					"kite": kite,
				},
			}
			stoppedMsgBytes, _ := json.Marshal(stoppedMsg)

			// notify kites of the same type
			for _, kiteID := range k.getIDsForKites(kite.Name) {
				k.Publish(kiteID, stoppedMsgBytes)
			}

			// then notify kites that depends on me..
			for _, c := range k.getRelationship(kite.Name) {
				k.Publish(c.ID, stoppedMsgBytes)
			}

			k.Publish("kite.start."+kite.Username, stoppedMsgBytes)

			// Am I the latest of my kind ? if yes remove me from the dependencies list
			// and remove any tokens if I have some
			if dependency.Has(kite.Name) {
				var found bool
				for _, t := range storage.List() {
					if t.Name == kite.Name {
						found = true
					}
				}

				if !found {
					dependency.Remove(kite.Name)
				}
			}
		}
	}
}

// handle handles the messages coming from Kites.
func (k *Kontrol) handle(msg []byte) ([]byte, error) {
	req, err := unmarshalRequest(msg)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("INCOMING KITE MSG req.Kite.ID: %+v req.Method: %+v\n", req.Kite.ID, req.Method)

	err = k.validateKiteRequest(req)
	if err != nil {
		return nil, error
	}

	// treat any incoming data as a ping, don't just rely on ping command
	// this makes the kite more robust if we can't catch one of the pings.
	k.updateKite(req.Kite.ID)

	switch req.Method {
	case protocol.Pong:
		return k.handlePong(req)
	case protocol.RegisterKite:
		return k.handleRegister(req)
	case protocol.GetKites:
		return k.handleGetKites(req)
	}

	return []byte("handle error"), nil
}

func (k *Kontrol) validateKiteRequest(req *protocol.KiteToKontrolRequest) error {
	username := usernameFromKey(req.KodingKey)
	if req.Kite.Username != usernameFromKey(key) {
		return errors.New("Invalid username")
	}

	kite := storage.Get(req.Kite.ID)
	if kite == nil {
		return nil
	}

	if req.Kite.ID != kite.ID {
		return errors.New("Invalid Kite ID")
	}

	return nil
}

func (k *Kontrol) updateKite(id string) error {
	kite := storage.Get(id)
	if kite == nil {
		return errors.New("not registered")
	}

	kite.UpdatedAt = time.Now().Add(protocol.HEARTBEAT_INTERVAL)
	storage.Add(kite)
	return nil
}

func unmarshalRequest(msg []byte) (*protocol.KiteToKontrolRequest, error) {
	req := new(protocol.KiteToKontrolRequest)
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (k *Kontrol) handlePong(req *protocol.KiteToKontrolRequest) ([]byte, error) {
	err := k.updateKite(req.Kite.ID)
	if err != nil {
		// happens when kite is not registered
		return []byte("UPDATE"), nil
	}

	return []byte("OK"), nil
}

func (k *Kontrol) handleRegister(req *protocol.KiteToKontrolRequest) ([]byte, error) {
	slog.Printf("[%s (%s)] at '%s' wants to be registered\n",
		req.Kite.Name, req.Kite.Version, req.Kite.Hostname)

	kite, err := k.RegisterKite(req)
	if err != nil {
		response := protocol.RegisterResponse{Result: protocol.RejectKite}
		resp, _ := json.Marshal(response)
		return resp, err
	}

	msg := newKiteMessageBytes(protocol.KiteRegistered, kite)

	// first notify myself
	k.Publish(req.Kite.ID, msg)

	// notify browser clients ...
	k.Publish("kite.start."+kite.Username, msg)

	// then notify dependencies of this kite, if any available
	k.NotifyDependencies(kite)

	startLog := fmt.Sprintf("[%s (%s)] starting at '%s' - '%s'",
		kite.Name,
		kite.Version,
		kite.Hostname,
		kite.ID,
	)
	slog.Println(startLog)

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResponse{
		Result:   protocol.AllowKite,
		Username: kite.Username,
	}

	resp, _ := json.Marshal(response)
	return resp, nil

}

func (k *Kontrol) handleGetKites(req *protocol.KiteToKontrolRequest) ([]byte, error) {
	username := req.Args["username"].(string)
	kitename := req.Args["kitename"].(string)

	kites, err := searchForKites(username, kitename)
	if err != nil {
		return nil, err
	}

	// Add myself as an dependency to the kite itself (to the kite I
	// request above). This is needed when new kites of that type appear
	// on kites that exist dissapear.
	slog.Printf("adding '%s' as a dependency to '%s' \n", req.Kite.Name, kitename)
	dependency.Add(kitename, req.Kite.Name)

	resp, err := json.Marshal(kites)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func newKiteMessageBytes(msgType protocol.MessageType, kite *models.Kite) []byte {
	// Cannot fail
	msg, _ := json.Marshal(newKiteMessage(msgType, kite))
	return msg
}

func newKiteMessage(msgType protocol.MessageType, kite *models.Kite) protocol.KontrolMessage {
	key, err := kodingkey.FromString(kite.KodingKey)
	if err != nil {
		// This cannot happen because we are not registering the kite
		// if it's koding key is invalid.
		panic(err)
	}

	// username is from requester, key is from kite owner
	tokenString, err := token.NewToken(kite.Username).EncryptString(key)
	if err != nil {
		panic(err)
	}

	msg := protocol.KontrolMessage{
		Type: msgType,
		Args: map[string]interface{}{
			"kite": kite.Kite,
		},
	}

	if msgType == protocol.KiteRegistered {
		msg.Args["token"] = tokenString
	}

	return msg
}

// Notifies all kites that depends on source kite, it may be kites of the same
// type (they have the same name) or kites that depens on it (like calles,
// clients or other kites of other types)
func (k *Kontrol) NotifyDependencies(kite *models.Kite) {
	// notify kites of the same type
	for _, r := range storage.List() {
		if r.Name == kite.Name && r.ID != kite.ID {
			// send other kites to me
			k.Publish(kite.ID, newKiteMessageBytes(protocol.KiteRegistered, r))

			// and then send myself to other kites. but don't send to
			// kite.Name, because it would send it again to me.
			k.Publish(r.ID, newKiteMessageBytes(protocol.KiteRegistered, kite))
		}
	}

	// notify myself to kites that depends on me
	for _, c := range k.getRelationship(kite.Name) {
		k.Publish(c.ID, newKiteMessageBytes(protocol.KiteRegistered, kite))
	}
}

func (k *Kontrol) Publish(filter string, msg []byte) {
	k.Publisher.Publish(filter, msg)
}

// RegisterKite returns true if the specified kite has been seen before.
// If not, it first validates the kites. If the kite has permission to run, it
// creates a new struct, stores it and returns it.
func (k *Kontrol) RegisterKite(req *protocol.KiteToKontrolRequest) (*models.Kite, error) {
	kite := storage.Get(req.Kite.ID)
	if kite != nil {
		return kite, nil
	}

	return createAndAddKite(req)
}

func createAndAddKite(req *protocol.KiteToKontrolRequest) (*models.Kite, error) {
	// in the future we'll check other things too, for now just make sure that
	// the variables are not empty
	if req.Kite.Name == "" && req.Kite.Version == "" && req.Kite.PublicIP == "" && req.Kite.Port == "" {
		return nil, fmt.Errorf("kite fields are not initialized correctly")
	}

	kite := createKiteModel(req)

	username, err := usernameFromKey(kite.KodingKey)
	if err != nil {
		return nil, err
	}

	kite.Username = username

	storage.Add(kite)

	slog.Printf("[%s (%s)] belong to '%s'. ready to go..\n", kite.Name, kite.Version, username)

	if req.Kite.Kind == "vm" {
		err := addToVM(username)
		if err != nil {
			fmt.Println("register get user id err")
		}
	}

	return kite, nil
}

func createKiteModel(req *protocol.KiteToKontrolRequest) *models.Kite {
	return &models.Kite{
		Kite:      req.Kite,
		KodingKey: req.KodingKey,
	}
}

func usernameFromKey(key string) (string, error) {
	kodingKey, err := modelhelper.GetKodingKeysByKey(key)
	if err != nil {
		return "", fmt.Errorf("register kodingkey err %s", err)
	}

	account, err := modelhelper.GetAccountById(kodingKey.Owner)
	if err != nil {
		return "", fmt.Errorf("register get user err %s", err)
	}

	if account.Profile.Nickname == "" {
		return "", errors.New("nickname is empty, could not register kite")
	}

	return account.Profile.Nickname, nil
}

func addToVM(username string) error {
	newVM := modelhelper.NewVM()
	newVM.HostnameAlias = "local-" + username
	newVM.IsEnabled = true
	newVM.WebHome = username

	user, err := modelhelper.GetUser(username)
	if err != nil {
		return err
	}

	newVM.Users = []models.Permissions{
		models.Permissions{
			Id:    user.ObjectId,
			Sudo:  true,
			Owner: true,
		}}

	group, err := modelhelper.GetGroup("Koding")
	if err != nil {
		return err
	}

	newVM.Groups = []models.Permissions{
		models.Permissions{
			Id: group.ObjectId,
		}}

	modelhelper.AddVM(&newVM)
	return nil
}

func deleteFromVM(username string) error {
	if username == "" {
		return errors.New("deleting local vm err: empty username is passed")
	}

	hostnameAlias := "local-" + username
	err := modelhelper.DeleteVM(hostnameAlias)
	if err != nil {
		return fmt.Errorf("deleting local vm err:", err)
	}
	return nil
}

func kitesExistsForUser(username string) bool {
	found := false

	for _, kite := range storage.List() {
		if kite.Username == username {
			found = true
		}
	}

	return found
}

// getRelationship returns a slice of of kites that has a relationship to kite itself.
func (k *Kontrol) getRelationship(kite string) []*models.Kite {
	targetKites := make([]*models.Kite, 0)

	for _, r := range storage.List() {
		for _, target := range dependency.List(kite) {
			if r.Name == target {
				targetKites = append(targetKites, r)
			}
		}
	}

	return targetKites
}

// getIDsForKites returns a list of ids collected from kites that matches
// the kitename argument.
func (k *Kontrol) getIDsForKites(kitename string) []string {
	ids := make([]string, 0)

	for _, s := range storage.List() {
		if s.Name == kitename {
			ids = append(ids, s.ID)
		}
	}

	return ids
}

// findUsernameFromSessionID reads the session id from websocket's
// protocol field and returns the username of the session.
func findUsernameFromSessionID(c *websocket.Config, r *http.Request) (string, error) {
	// return an empty string if session id is not sent
	if len(c.Protocol) < 1 {
		return "", nil
	}
	sessionID := c.Protocol[0]

	session, err := modelhelper.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	slog.Println("Websocket is authenticated as:", session.Username)

	return session.Username, nil
}

func validateCommand(username string, cmd *moh.SubscriberCommand) bool {
	if cmd.Name != "subscribe" || cmd.Name != "unsubscribe" {
		return true
	}

	key := cmd.Args["key"].(string)

	if !strings.HasPrefix(key, "kite.start.") {
		return true
	}
	if strings.TrimPrefix(key, "kite.start.") != username {
		return false
	}

	return true
}

func addToProxy(kite *models.Kite) {
	err := utils.IsServerAlive(kite.Addr())
	if err != nil {
		slog.Printf("server not reachable: %s (%s) \n", kite.Addr(), err.Error())
	} else {
		slog.Println("checking ok..", kite.Addr())
	}

	err = modelhelper.UpsertKey(
		kite.Username,     // username
		"",                // persistence, empty means disabled
		"",                // loadbalancing mode, empty means direct
		kite.Name,         // servicename
		kite.Version,      // key
		kite.Addr(),       // host
		"FromKontrolKite", // hostdata
		"",                // rabbitkey, not used currently
	)
	if err != nil {
		slog.Println("err")
	}

}
