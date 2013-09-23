package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	uuid "github.com/nu7hatch/gouuid"
	zmq "github.com/pebbe/zmq3"
	"io"
	"io/ioutil"
	"koding/db/models"
	"koding/db/mongodb"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/protocol"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"log"
	"net"
	"net/http"
	"os"
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
	Publisher *zmq.Socket
	Router    *zmq.Socket
	PubAddr   string
	RepAddr   string
	Hostname  string
}

var (
	self       string
	tokens     = make(map[string]*protocol.Token)
	storage    Storage
	dependency Dependency
)

func main() {
	router, _ := zmq.NewSocket(zmq.ROUTER)
	router.Bind("tcp://*:5556")

	pub, _ := zmq.NewSocket(zmq.PUB)
	pub.Bind("tcp://*:5557")

	hostname, _ := os.Hostname()

	// storage = peers.New()  // in-memory, map based, non-persistence storage
	// storage = NewRethinkDB() // future
	// storage = NewRedis() // future
	storage = NewMongoDB()
	dependency = NewDependency()

	k := &Kontrol{
		Hostname:  hostname,
		Publisher: pub,
		Router:    router,
	}

	fmt.Println("kontrol started")
	k.Start()
}

func (k *Kontrol) Start() {
	// This is used for two reasons
	// 1. HeartBeat mechanism for kite (Node Coordination)
	// 2. Triggering kites to register themself to kontrol (Synchronize PUB/SUB)
	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	go func() {
		for _ = range ticker.C {
			k.Ping()
		}
	}()

	// HeartBeat pool checker. Checking for kites if they are live or dead.
	go k.HeartBeatChecker()

	go func() {
		for {
			msg, _ := k.Router.RecvMessageBytes(0)
			if len(msg) != 3 { // msg is malformed
				continue
			}

			identity := msg[0]
			result, err := k.handle(msg[2])
			if err != nil {
				log.Println(err)
			}

			k.Router.SendBytes(identity, zmq.SNDMORE)
			k.Router.SendBytes([]byte(""), zmq.SNDMORE)
			k.Router.SendBytes(result, 0)
		}
	}()

	rout := mux.NewRouter()
	rout.HandleFunc("/", home).Methods("GET")
	rout.HandleFunc("/ip", ip).Methods("GET")
	rout.HandleFunc("/request", request).Methods("POST")
	http.Handle("/", rout)
	log.Println(http.ListenAndServe(":4000", nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world - kontrol!\n")
}

func ip(w http.ResponseWriter, r *http.Request) {
	ip := getIP(r.RemoteAddr)
	io.WriteString(w, ip)
}

func request(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GOT A REQUEST")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var msg protocol.Request

	body, _ := ioutil.ReadAll(r.Body)
	err := json.Unmarshal(body, &msg)
	if err != nil {
		http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
		return
	}

	fmt.Printf("request  %+v\n", msg)

	s, err := getSession(msg.SessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
		return
	}

	if s.Username != msg.Username || s.Username == "" {
		http.Error(w, "{\"err\":\"not authorized 2\"}\n", http.StatusBadRequest)
		return
	}

	if s.ClientId != msg.SessionID {
		http.Error(w, "{\"err\":\"not authorized 3\"}\n", http.StatusBadRequest)
		return
	}

	fmt.Printf("you are validated %s\n", s.Username)

	list := make([]protocol.PubResponse, 0)
	for _, k := range storage.List() {
		if k.Kitename == msg.RemoteKite {
			var token *protocol.Token
			token = getToken(s.Username)
			if token == nil {
				token = createToken(s.Username)
			}

			k.Token = token.ID // only token id is important for requester
			pubResp := createResponse(protocol.AddKite, k)
			list = append(list, pubResp)
		}
	}

	if len(list) == 0 {
		http.Error(w, "{\"err\":\"not kites available\"}\n", http.StatusBadRequest)
		return
	}

	l, err := json.Marshal(list)
	if err != nil {
		fmt.Println("marshalling kite list:", err)
		http.Error(w, "{\"err\":\"not authorized 4\"}\n", http.StatusBadRequest)
		return
	}

	w.Write([]byte(l))
}

func (k *Kontrol) Ping() {
	m := protocol.Request{
		Base: protocol.Base{
			Hostname: k.Hostname,
		},
		Action: "ping",
	}

	msg, _ := json.Marshal(&m)
	k.Publish("all", msg, false)
}

func (k *Kontrol) HeartBeatChecker() {
	ticker := time.NewTicker(protocol.HEARTBEAT_INTERVAL)
	go func() {
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
				fmt.Println(removeLog)

				storage.Remove(kite.Uuid)

				// notify kites of the same type
				pubResp := createResponse(protocol.RemoveKite, kite)
				msg, _ := json.Marshal(pubResp)
				k.Publish(kite.Kitename, msg, false)

				// then notify kites that depends on me..
				for _, c := range k.getRelationship(kite.Kitename) {
					k.Publish(c.Kitename, msg, false)
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
	}()
}

func (k *Kontrol) handle(msg []byte) ([]byte, error) {
	// rest we assume that it complies with our protocol
	var req protocol.Request
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, err
	}

	switch req.Action {
	case "pong":
		err := k.UpdateKite(req.Uuid)
		if err != nil {
			return []byte("UPDATE"), nil
		}

		return []byte("OK"), nil
	case "register":
		kite, err := k.RegisterKite(req)
		if err != nil {
			response := protocol.RegisterResponse{Addr: self, Result: protocol.PermitKite}
			resp, _ := json.Marshal(response)
			return resp, err
		}

		k.UpdateKite(req.Uuid)

		// disable this for now
		// go addToProxy(kite)

		// first notify myself
		pubResp := createResponse(protocol.AddKite, kite)
		msg, _ := json.Marshal(pubResp)
		k.Publish(req.Uuid, msg, false)

		// then notify dependencies of this kite, if any available
		k.NotifyDependencies(kite)

		startLog := fmt.Sprintf("[%s (%s)] starting at '%s' - '%s'",
			kite.Kitename,
			kite.Version,
			kite.Hostname,
			kite.Uuid,
		)
		fmt.Println(startLog)

		// send response back that we published the necessary informations
		response := protocol.RegisterResponse{
			Addr:   self,
			Result: protocol.AllowKite,
		}
		resp, _ := json.Marshal(response)
		return resp, nil
	case "getKites":
		fmt.Println("getKites request from: ", string(msg))
		k.UpdateKite(req.Uuid)

		// publish all remoteKites to me, with a token appended to them
		for _, r := range storage.List() {
			if r.Kitename == req.RemoteKite {
				pubResp := createResponse(protocol.AddKite, r)
				msg, _ := json.Marshal(pubResp)
				k.Publish(req.Uuid, msg, true)
			}
		}

		// Add myself as an dependency to the kite itself (to the kite I
		// request above). This is needed when new kites of that type appear
		// on kites that exist dissapear.
		fmt.Printf("adding '%s' as a dependency to '%s' \n", req.Kitename, req.RemoteKite)
		dependency.Add(req.RemoteKite, req.Kitename)

		resp, err := json.Marshal(protocol.RegisterResponse{Addr: self, Result: "kitesPublished"})
		if err != nil {
			return nil, err
		}

		return resp, nil
	case "getPermission":
		fmt.Println("getPermission request from: ", string(msg))
		k.UpdateKite(req.Uuid)

		msg := protocol.RegisterResponse{}

		token := getToken(req.Username)
		if token == nil || token.ID != req.Token {
			msg = protocol.RegisterResponse{Addr: self, Result: protocol.PermitKite}
		} else {
			msg = protocol.RegisterResponse{Addr: self, Result: protocol.AllowKite, Token: *token}
		}

		resp, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}

		return resp, nil
	}

	return []byte("handle error"), nil
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
			// TODO: also send the len and compare it on the kite side
			pubResp := createResponse(protocol.AddKite, r)
			msg, _ := json.Marshal(pubResp)
			k.Publish(kite.Uuid, msg, true)

			// and then send myself to other kites
			pubResp = createResponse(protocol.AddKite, kite)
			msg, _ = json.Marshal(pubResp)
			k.Publish(r.Uuid, msg, true) // don't send to kite.Kitename, it would send it also again to me
		}
	}

	// notify myself to kites that depends on me, attach also a token if I have one
	for _, c := range k.getRelationship(kite.Kitename) {
		pubResp := createResponse(protocol.AddKite, kite)
		msg, _ := json.Marshal(pubResp)
		k.Publish(c.Uuid, msg, true)
	}
}

func (k *Kontrol) Publish(filter string, msg []byte, logEnabled bool) {
	msg = []byte(filter + protocol.FRAME_SEPARATOR + string(msg))
	if logEnabled {
		fmt.Printf("pub send: %s\n", string(msg))
	}

	k.Publisher.SendBytes(msg, 0)
}

// RegisterKite returns true if the specified kite has been seen before.
// If not, it first validates the kites. If the kite has permission to run, it
// creates a new struct, stores it and returns it.
func (k *Kontrol) RegisterKite(req protocol.Request) (*models.Kite, error) {
	kite := storage.Get(req.Uuid)
	if kite == nil {
		kite = &models.Kite{
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
		}

		if !validate(kite) {
			return nil, fmt.Errorf("kite %s - %s is not validated", req.Kitename, req.Uuid)
		}

		startLog := fmt.Sprintf("[%s (%s)] public key '%s' is registered. ready to go..",
			kite.Kitename,
			kite.Version,
			kite.PublicKey,
		)
		fmt.Println(startLog)

		storage.Add(kite)
	}
	return kite, nil
}

func (k *Kontrol) UpdateKite(Uuid string) error {
	kite := storage.Get(Uuid)
	if kite == nil {
		return errors.New("not registered")
	}
	kite.UpdatedAt = time.Now().Add(protocol.HEARTBEAT_INTERVAL)
	storage.Add(kite)
	return nil
}

// GetRelationship returns a slice of of kites that has a relationship to kite itself
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

func GenerateToken() string {
	id, _ := uuid.NewV4()
	return id.String()
}

// for now these fields are enough
type Session struct {
	Id       bson.ObjectId `bson:"_id" json:"-"`
	ClientId string        `bson:"clientId"`
	Username string        `bson:"username"`
	GuestId  int           `bson:"guestId"`
}

func getSession(token string) (*Session, error) {
	var session *Session

	query := func(c *mgo.Collection) error {
		return c.Find(bson.M{"clientId": token}).One(&session)
	}

	err := mongodb.Run("jSessions", query)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// for now these fields are enough
type KodingKey struct {
	Id       bson.ObjectId `bson:"_id" json:"-"`
	Key      string        `bson:"key"`
	Hostname string        `bson:"hostname"`
	Owner    string        `bson:"owner"`
}

// check whether the publicKey is available (registered) or not. return true if
// available
func checkKey(publicKey string) bool {
	kodingKey := &KodingKey{}
	query := func(c *mgo.Collection) error {
		return c.Find(bson.M{"key": publicKey}).One(&kodingKey)
	}

	err := mongodb.Run("jKodingKeys", query)
	if err != nil {
		fmt.Println("public key is not registered", publicKey)
		return false
	}

	return true
}

func validate(k *models.Kite) bool {
	// in the future we'll check other things too, for now just make sure that
	// the variables are not empty
	if k.Username == "" && k.Kitename == "" && k.Version == "" && k.Addr == "" {
		return false
	}

	// check if public key has permission to run
	return checkKey(k.PublicKey)
}

func getIP(addr string) string {
	ip, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return ip
}

func checkServer(host string) error {
	fmt.Println("checking host", host)
	c, err := net.DialTimeout("tcp", host, time.Second*5)
	if err != nil {
		return err
	}
	c.Close()
	return nil
}

func addToProxy(kite *models.Kite) {
	err := checkServer(kite.Addr)
	if err != nil {
		fmt.Printf("server not reachable: %s (%s) \n", kite.Addr, err.Error())
	} else {
		fmt.Println("checking ok..", kite.Addr)
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
		log.Println("err")
	}

}

func NewToken(username string) *protocol.Token {
	return &protocol.Token{
		ID:        GenerateToken(),
		Username:  username,
		Expire:    0,
		CreatedAt: time.Now(),
	}
}

func getToken(username string) *protocol.Token {
	token, ok := tokens[username]
	if !ok {
		return nil
	}

	return token
}

func createToken(username string) *protocol.Token {
	t := NewToken(username)
	tokens[username] = t
	return t
}

func deleteToken(username string) {
	delete(tokens, username)
}
