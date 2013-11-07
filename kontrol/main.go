package main

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	logging "github.com/op/go-logging"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"koding/messaging/moh"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"koding/tools/config"
	stdlog "log"
	"net"
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
	log = logging.MustGetLogger("Kontrol")

	storage    Storage
	dependency Dependency
)

const SubscribePrefix = "kite."

func main() {
	hostname, _ := os.Hostname()

	k := &Kontrol{
		Hostname: hostname,
		Port:     strconv.Itoa(config.Current.NewKontrol.Port),
	}

	k.Replier = moh.NewReplier(k.replyMohRequest)
	k.Publisher = moh.NewPublisher()
	k.Publisher.Authenticate = findUsernameFromSessionID
	k.Publisher.ValidateCommand = validateCommand

	storage = NewMongoDB()
	dependency = NewDependency()

	k.setupLogging()

	k.Start()
}

func (k *Kontrol) setupLogging() {
	log.Module = "Kontrol"
	logging.SetFormatter(logging.MustStringFormatter("%{level:-8s} ▶ %{message}"))
	stderrBackend := logging.NewLogBackend(os.Stderr, "", stdlog.LstdFlags|stdlog.Lshortfile)
	stderrBackend.Color = true
	syslogBackend, _ := logging.NewSyslogBackend(log.Module)
	logging.SetBackend(stderrBackend, syslogBackend)

	log.Info("started")
}

func (k *Kontrol) Start() {
	// scheduler functions
	go k.pinger()
	go k.heartBeatChecker()
	rout := mux.NewRouter()
	rout.HandleFunc("/", homeHandler).Methods("GET")
	rout.HandleFunc("/query", errHandler(queryHandler)).Methods("POST")
	rout.Handle(moh.DefaultReplierPath, k.Replier)
	rout.Handle(moh.DefaultPublisherPath, k.Publisher)
	http.Handle("/", rout)

	log.Error(http.ListenAndServe(":"+k.Port, nil).Error())
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
	k.Publisher.Broadcast(msg)
}

// HeartBeat pool checker. Checking for kites if they are live or dead.
// It removes kites from the DB if they are no more alive.
func (k *Kontrol) heartBeatChecker() {
	// Wait for a while before removing dead kites.
	// It is required to maintain old kites on db in case of Kontrol restart.
	time.Sleep(protocol.HEARTBEAT_DELAY)

	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	for _ = range ticker.C {
		for _, kite := range storage.List() {
			// Delay is needed to fix network delays, otherwise kites are
			// marked as death even if they are sending 'pongs' to us
			if time.Now().UTC().Before(kite.UpdatedAt.Add(protocol.HEARTBEAT_DELAY)) {
				continue // still alive, pick up the next one
			}

			removeLog := fmt.Sprintf("[%s (%s)] dead at '%s' - '%s'",
				kite.Name,
				kite.Version,
				kite.Hostname,
				kite.ID,
			)
			log.Info(removeLog)

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

			k.Publish(SubscribePrefix+kite.Username, stoppedMsgBytes)

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

// replyMohRequest handles the messages coming from Kites.
func (k *Kontrol) replyMohRequest(httpReq *http.Request, msg []byte) ([]byte, error) {
	req, err := unmarshalRequest(msg)
	if err != nil {
		return nil, err
	}
	// log.Debug("INCOMING KITE MSG req.Kite.ID: %#v req.Method: %#v", req.Kite.ID, req.Method)

	err = k.validateKiteRequest(req)
	if err != nil {
		return nil, err
	}

	// treat any incoming data as a ping, don't just rely on ping command
	// this makes the kite more robust if we can't catch one of the pings.
	k.updateKite(req.Kite.ID)

	switch req.Method {
	case protocol.Pong:
		return k.handlePong(req)
	case protocol.RegisterKite:
		return k.handleRegister(httpReq, req)
	case protocol.GetKites:
		return k.handleGetKites(req)
	}

	return nil, errors.New("Invalid method")
}

func (k *Kontrol) validateKiteRequest(req *protocol.KiteToKontrolRequest) error {
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

	kite.UpdatedAt = time.Now().UTC().Add(protocol.HEARTBEAT_INTERVAL)
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

func (k *Kontrol) handleRegister(httpReq *http.Request, req *protocol.KiteToKontrolRequest) ([]byte, error) {
	log.Info("[%s (%s)] at '%s' wants to be registered",
		req.Kite.Name, req.Kite.Version, req.Kite.Hostname)

	remoteHost, _, _ := net.SplitHostPort(httpReq.RemoteAddr)

	kite, err := k.RegisterKite(req, remoteHost)
	if err != nil {
		response := protocol.RegisterResponse{Result: protocol.RejectKite}
		resp, _ := json.Marshal(response)
		return resp, err
	}

	msg := newKiteMessageBytes(protocol.KiteRegistered, kite)

	// first notify myself
	k.Publish(req.Kite.ID, msg)

	// notify browser clients ...
	k.Publish(SubscribePrefix+kite.Username, msg)

	// then notify dependencies of this kite, if any available
	k.NotifyDependencies(kite)

	startLog := fmt.Sprintf("[%s (%s)] starting at '%s' - '%s'",
		kite.Name,
		kite.Version,
		kite.Hostname,
		kite.ID,
	)
	log.Info(startLog)

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResponse{
		Result:   protocol.AllowKite,
		Username: kite.Username,
		PublicIP: remoteHost,
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
	log.Info("adding '%s' as a dependency to '%s'", req.Kite.Name, kitename)
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
	// Error is omitted because we do not register kites with invalid ID
	key, _ := kodingkey.FromString(kite.KodingKey)

	// username is from requester, key is from kite owner
	tokenString, _ := token.NewToken(kite.Username, kite.ID).EncryptString(key)

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
func (k *Kontrol) RegisterKite(req *protocol.KiteToKontrolRequest, ip string) (*models.Kite, error) {
	if req.Kite.ID == "" {
		return nil, errors.New("Invalid Kite ID")
	}

	if req.KodingKey == "" {
		return nil, errors.New("Invalid Koding Key")
	}

	kite := storage.Get(req.Kite.ID)
	if kite != nil {
		return kite, nil
	}

	return createAndAddKite(req, ip)
}

func createAndAddKite(req *protocol.KiteToKontrolRequest, remoteIP string) (*models.Kite, error) {
	// in the future we'll check other things too, for now just make sure that
	// the variables are not empty
	if req.Kite.Name == "" && req.Kite.Version == "" && req.Kite.PublicIP == "" && req.Kite.Port == "" {
		return nil, fmt.Errorf("kite fields are not initialized correctly")
	}

	kite := modelhelper.NewKite()
	kite.Kite = req.Kite
	kite.KodingKey = req.KodingKey

	if req.Kite.PublicIP == "" {
		kite.PublicIP = remoteIP
	}

	username, err := usernameFromKey(kite.KodingKey)
	if err != nil {
		return nil, err
	}

	kite.Username = username

	storage.Add(kite)

	log.Info("[%s (%s)] belong to '%s'. ready to go..", kite.Name, kite.Version, username)
	return kite, nil
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
	log.Info("Websocket is authenticated as: %s", session.Username)

	return session.Username, nil
}

func validateCommand(username string, cmd *moh.SubscriberCommand) bool {
	// Return if incoming is not one of subscribe or unsubscribe
	if cmd.Name != "subscribe" && cmd.Name != "unsubscribe" {
		return true
	}

	key := cmd.Args["key"].(string)

	// if it has doesn't have prefix let im trough
	if !strings.HasPrefix(key, SubscribePrefix) {
		return true
	}

	// now check if "kite.usernamefield" really is the same with the requester
	// username. Users shouldn't be able subscribe to other people's kites.
	// the `username` is fetched via websocket protocol authentication.
	if strings.TrimPrefix(key, SubscribePrefix) != username {
		return false
	}

	return true
}
