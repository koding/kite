package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"koding/messaging/moh"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"koding/tools/config"
	"koding/tools/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Storage is an interface that encapsulates basic operations on the kite
// struct. Uuid is an unique string that belongs to the kite.
type Storage interface {
	// Add inserts the kite into the storage with the kite.Uuid key. If there
	// is already a kite available with this uuid, it should update/replace it.
	Add(kite *models.Kite)

	// Get returns the specified kite struct with the given uuid
	Get(uuid string) *models.Kite

	// Remove deletes the kite with the given uuid
	Remove(uuid string)

	// Has checks whether the kite with the given uuid exist
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
	self       string
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

	storage = NewMongoDB()
	dependency = NewDependency()

	slog.SetPrefixName("kontrol")
	slog.SetPrefixTimeStamp(time.Stamp)
	slog.Println("started")

	k.Start()
}

func (k *Kontrol) Start() {
	// ping all subscribers
	go k.pinger()

	// HeartBeat pool checker. Checking for kites if they are live or dead.
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

func (k *Kontrol) pinger() {
	// This is used for two reasons
	// 1. HeartBeat mechanism for kite (Node Coordination)
	// 2. Triggering kites to register themself to kontrol (Synchronize PUB/SUB)
	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	for _ = range ticker.C {
		k.ping()
	}
}

func (k *Kontrol) ping() {
	m := protocol.Request{
		Base: protocol.Base{
			Hostname: k.Hostname,
		},
		Action: "ping",
	}

	msg, _ := json.Marshal(&m)
	k.Publish("all", msg)
}

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
				kite.Kitename,
				kite.Version,
				kite.Hostname,
				kite.Uuid,
			)
			slog.Println(removeLog)

			storage.Remove(kite.Uuid)

			if !kitesBelongingTo(kite.Username) {
				deleteFromVM(kite.Username)
			}

			removeMsg := createByteResponse(protocol.RemoveKite, kite)
			// notify kites of the same type
			for _, kiteUUID := range k.getUUIDsForKites(kite.Kitename) {
				k.Publish(kiteUUID, removeMsg)
			}

			// then notify kites that depends on me..
			for _, c := range k.getRelationship(kite.Kitename) {
				k.Publish(c.Uuid, removeMsg)
			}

			// Am I the latest of my kind ? if yes remove me from the dependencies list
			// and remove any tokens if I have some
			if dependency.Has(kite.Kitename) {
				var found bool
				for _, t := range storage.List() {
					if t.Kitename == kite.Kitename {
						found = true
					}
				}

				if !found {
					deleteToken(kite.Kitename)
					dependency.Remove(kite.Kitename)
				}
			}
		}
	}
}

func (k *Kontrol) handle(msg []byte) ([]byte, error) {
	req, err := unmarshalRequest(msg)
	if err != nil {
		return nil, err
	}

	k.updateKite(req.Uuid)

	switch req.Action {
	case "pong":
		return k.handlePong(req)
	case "register":
		return k.handleRegister(req)
	case "getKites":
		return k.handleGetKites(req)
	case "getPermission":
		return k.handleGetPermission(req)
	}

	return []byte("handle error"), nil
}

func (k *Kontrol) updateKite(Uuid string) error {
	kite := storage.Get(Uuid)
	if kite == nil {
		return errors.New("not registered")
	}

	kite.UpdatedAt = time.Now().Add(protocol.HEARTBEAT_INTERVAL)
	storage.Add(kite)
	return nil
}

func unmarshalRequest(msg []byte) (*protocol.Request, error) {
	req := new(protocol.Request)
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (k *Kontrol) handlePong(req *protocol.Request) ([]byte, error) {
	err := k.updateKite(req.Uuid)
	if err != nil {
		return []byte("UPDATE"), nil
	}

	return []byte("OK"), nil
}

func (k *Kontrol) handleRegister(req *protocol.Request) ([]byte, error) {
	slog.Printf("[%s (%s)] at '%s' wants to be registered\n",
		req.Kitename, req.Version, req.Hostname)

	kite, err := k.RegisterKite(req)
	if err != nil {
		response := protocol.RegisterResponse{Addr: self, Result: protocol.PermitKite}
		resp, _ := json.Marshal(response)
		return resp, err
	}

	// disable this for now
	// go addToProxy(kite)

	// first notify myself
	k.Publish(req.Uuid, createByteResponse(protocol.AddKite, kite))

	// then notify dependencies of this kite, if any available
	k.NotifyDependencies(kite)

	startLog := fmt.Sprintf("[%s (%s)] starting at '%s' - '%s'",
		kite.Kitename,
		kite.Version,
		kite.Hostname,
		kite.Uuid,
	)
	slog.Println(startLog)

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResponse{
		Addr:     self,
		Result:   protocol.AllowKite,
		Username: kite.Username,
	}

	resp, _ := json.Marshal(response)
	return resp, nil

}
func (k *Kontrol) handleGetKites(req *protocol.Request) ([]byte, error) {
	kites, err := searchForKites(req.Username, req.RemoteKite)
	if err != nil {
		return nil, err
	}

	// Add myself as an dependency to the kite itself (to the kite I
	// request above). This is needed when new kites of that type appear
	// on kites that exist dissapear.
	slog.Printf("adding '%s' as a dependency to '%s' \n", req.Kitename, req.RemoteKite)
	dependency.Add(req.RemoteKite, req.Kitename)

	resp, err := json.Marshal(kites)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (k *Kontrol) handleGetPermission(req *protocol.Request) ([]byte, error) {
	slog.Printf("[%s] asks if token '%s' is valid\n", req.Kitename, req.Token)

	msg := protocol.RegisterResponse{}

	token := getToken(req.Username)
	if token == nil || token.ID != req.Token {
		slog.Printf("token '%s' is invalid for '%s' \n", req.Token, req.Kitename)
		msg = protocol.RegisterResponse{Addr: self, Result: protocol.PermitKite}
	} else {
		slog.Printf("token '%s' is valid for '%s' \n", req.Token, req.Kitename)
		msg = protocol.RegisterResponse{Addr: self, Result: protocol.AllowKite, Token: *token}
	}

	resp, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func createByteResponse(action string, kite *models.Kite) []byte {
	msg, _ := json.Marshal(createResponse(action, kite))
	return msg // no way that this can produce an error
}

func createResponse(action string, kite *models.Kite) protocol.PubResponse {
	return protocol.PubResponse{
		Base: protocol.Base{
			Username: kite.Username,
			Kitename: kite.Kitename,
			Version:  kite.Version,
			Uuid:     kite.Uuid,
			Token:    kite.Token,
			Hostname: kite.Hostname,
			Addr:     kite.Addr,
			LocalIP:  kite.LocalIP,
			PublicIP: kite.PublicIP,
			Port:     kite.Port,
		},
		Action: action,
	}
}

// Notifies all kites that depends on source kite, it may be kites of the same
// type (they have the same name) or kites that depens on it (like calles,
// clients or other kites of other types)
func (k *Kontrol) NotifyDependencies(kite *models.Kite) {
	// notify kites of the same type
	for _, r := range storage.List() {
		if r.Kitename == kite.Kitename && r.Uuid != kite.Uuid {
			// send other kites to me
			k.Publish(kite.Uuid, createByteResponse(protocol.AddKite, r))

			// and then send myself to other kites. but don't send to
			// kite.Kitename, because it would send it again to me.
			k.Publish(r.Uuid, createByteResponse(protocol.AddKite, kite))
		}
	}

	// notify myself to kites that depends on me
	for _, c := range k.getRelationship(kite.Kitename) {
		k.Publish(c.Uuid, createByteResponse(protocol.AddKite, kite))
	}
}

func (k *Kontrol) Publish(filter string, msg []byte) {
	k.Publisher.Publish(filter, msg)
}

// RegisterKite returns true if the specified kite has been seen before.
// If not, it first validates the kites. If the kite has permission to run, it
// creates a new struct, stores it and returns it.
func (k *Kontrol) RegisterKite(req *protocol.Request) (*models.Kite, error) {
	kite := storage.Get(req.Uuid)
	if kite != nil {
		return kite, nil
	}

	return createAndAddKite(req)
}

func createAndAddKite(req *protocol.Request) (*models.Kite, error) {
	// in the future we'll check other things too, for now just make sure that
	// the variables are not empty
	if req.Kitename == "" && req.Version == "" && req.Addr == "" {
		return nil, fmt.Errorf("kite fields are not initialized correctly")
	}

	kite, err := createKiteModel(req)
	if err != nil {
		return nil, err
	}

	username, err := usernameFromKey(kite.PublicKey)
	if err != nil {
		return nil, err
	}

	kite.Username = username
	storage.Add(kite)

	slog.Printf("[%s (%s)] belong to '%s'. ready to go..\n", kite.Kitename, kite.Version, username)

	if req.Kind == "vm" {
		err := addToVM(username)
		if err != nil {
			fmt.Println("register get user id err")
		}
	}

	return kite, nil
}

func createKiteModel(req *protocol.Request) (*models.Kite, error) {
	return &models.Kite{
		Base: protocol.Base{
			Username:  req.Username,
			Kitename:  req.Kitename,
			Version:   req.Version,
			PublicKey: req.PublicKey,
			Uuid:      req.Uuid,
			Hostname:  req.Hostname,
			Addr:      req.Addr,
			LocalIP:   req.LocalIP,
			PublicIP:  req.PublicIP,
			Port:      req.Port,
		},
	}, nil
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

func kitesBelongingTo(username string) bool {
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
	if storage.Size() == 0 {
		return targetKites
	}

	for _, r := range storage.List() {
		for _, target := range dependency.List(kite) {
			if r.Kitename == target {
				targetKites = append(targetKites, r)
			}
		}
	}

	return targetKites
}

// getUUIDsForKites returns a list of uuids collected from kites that matches
// the kitename argument.
func (k *Kontrol) getUUIDsForKites(kitename string) []string {
	uuids := make([]string, 0)

	for _, s := range storage.List() {
		if s.Kitename == kitename {
			uuids = append(uuids, s.Uuid)
		}
	}

	return uuids
}

func addToProxy(kite *models.Kite) {
	err := utils.IsServerAlive(kite.Addr)
	if err != nil {
		slog.Printf("server not reachable: %s (%s) \n", kite.Addr, err.Error())
	} else {
		slog.Println("checking ok..", kite.Addr)
	}

	err = modelhelper.UpsertKey(
		kite.Username,     // username
		"",                // persistence, empty means disabled
		"",                // loadbalancing mode, empty means direct
		kite.Kitename,     // servicename
		kite.Version,      // key
		kite.Addr,         // host
		"FromKontrolKite", // hostdata
		"",                // rabbitkey, not used currently
	)
	if err != nil {
		slog.Println("err")
	}

}
