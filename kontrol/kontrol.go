package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"github.com/op/go-logging"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/dnode"
	"koding/newkite/kite"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"koding/tools/config"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	HeartbeatInterval = 1 * time.Minute
	HeartbeatDelay    = 2 * time.Minute
	KitesPrefix       = "/kites"
)

var log *logging.Logger

type Kontrol struct {
	kite       *kite.Kite
	etcd       *etcd.Client
	watcherHub *watcherHub
}

func New() *Kontrol {
	kiteOptions := &kite.Options{
		Kitename:    "kontrol",
		Version:     "1",
		Port:        strconv.Itoa(config.Current.NewKontrol.Port),
		Region:      "localhost",
		Environment: "development",
	}

	// Read list of etcd servers from config.
	machines := make([]string, len(config.Current.Etcd))
	for i, s := range config.Current.Etcd {
		machines[i] = "http://" + s.Host + ":" + strconv.FormatUint(uint64(s.Port), 10)
	}

	kontrol := &Kontrol{
		kite:       kite.New(kiteOptions),
		etcd:       etcd.NewClient(machines),
		watcherHub: newWatcherHub(),
	}

	log = kontrol.kite.Log

	kontrol.kite.KontrolEnabled = false // Because we are Kontrol!

	kontrol.kite.Authenticators["kodingKey"] = kontrol.AuthenticateFromKodingKey
	kontrol.kite.Authenticators["sessionID"] = kontrol.AuthenticateFromSessionID

	kontrol.kite.HandleFunc("register", kontrol.handleRegister)
	kontrol.kite.HandleFunc("getKites", kontrol.handleGetKites)
	kontrol.kite.HandleFunc("getToken", kontrol.handleGetToken)

	return kontrol
}

func (k *Kontrol) Run() {
	k.init()
	k.kite.Run()
}

func (k *Kontrol) Start() {
	k.init()
	k.kite.Start()
}

// init does common operations of Run() and Start().
func (k *Kontrol) init() {
	go k.WatchEtcd()
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	PublicIP  string
	Port      string
	KodingKey string
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %#v", r.RemoteKite.Kite)

	// Only accept requests with kodingKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kodingKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	// Set PublicIP address if it's empty.
	if r.RemoteKite.PublicIP == "" {
		r.RemoteKite.PublicIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	return k.register(r.RemoteKite, r.Authentication.Key)
}

func (k *Kontrol) register(r *kite.RemoteKite, kodingkey string) (*protocol.RegisterResult, error) {
	kite := &r.Kite

	key, err := getKiteKey(kite)
	if err != nil {
		return nil, err
	}

	// setKey sets the value of the Kite in etcd.
	setKey := k.makeSetter(kite, key, kodingkey)

	// Register to etcd.
	prev, err := setKey()
	if err != nil {
		return nil, errors.New("Internal error")
	}

	if prev != "" {
		log.Notice("Kite (%s) is already registered. Doing nothing.", key)
	} else if err := requestHeartbeat(r, setKey); err != nil {
		return nil, err
	}

	log.Info("Kite registered: %s", key)
	k.watcherHub.Notify(kite, protocol.Register, kodingkey)

	r.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		k.etcd.Delete(key)
	})

	// send response back to the kite, also identify him with the new name
	return &protocol.RegisterResult{
		Result:   protocol.AllowKite,
		Username: r.Username,
		PublicIP: r.PublicIP,
	}, nil
}

func requestHeartbeat(r *kite.RemoteKite, setterFunc func() (string, error)) error {
	heartbeatFunc := func(p *dnode.Partial) {
		prev, err := setterFunc()
		if err == nil && prev == "" {
			log.Warning("Came heartbeat but the Kite (%s) is not registered. Re-registering it. It may be an indication that the heartbeat delay is too short.", r.ID)
		}
	}

	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		dnode.Callback(heartbeatFunc),
	}

	_, err := r.Call("heartbeat", heartbeatArgs)
	return err
}

//  makeSetter returns a func for setting the kite key with value in etcd.
func (k *Kontrol) makeSetter(kite *protocol.Kite, etcdKey, kodingkey string) func() (string, error) {
	rv := &registerValue{
		PublicIP:  kite.PublicIP,
		Port:      kite.Port,
		KodingKey: kodingkey,
	}

	valueBytes, _ := json.Marshal(rv)
	value := string(valueBytes)

	ttl := uint64(HeartbeatDelay / time.Second)

	return func() (prevValue string, err error) {
		resp, err := k.etcd.Set(etcdKey, value, ttl)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return
		}

		prevValue = resp.PrevValue

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		_, err = k.etcd.UpdateDir(KitesPrefix+"/"+kite.Username, ttl)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return
		}

		return
	}
}

// getKiteKey returns a string representing the kite uniquely
// that is suitable to use as a key for etcd.
func getKiteKey(k *protocol.Kite) (string, error) {
	// Order is important.
	fields := map[string]string{
		"username":    k.Username,
		"environment": k.Environment,
		"name":        k.Name,
		"version":     k.Version,
		"region":      k.Region,
		"hostname":    k.Hostname,
		"id":          k.ID,
	}

	// Validate fields.
	for k, v := range fields {
		if v == "" {
			return "", fmt.Errorf("Empty Kite field: %s", k)
		}
		if strings.ContainsRune(v, '/') {
			return "", fmt.Errorf("Field \"%s\" must not contain '/'", k)
		}
	}

	// Build key.
	key := "/"
	for _, v := range fields {
		key = key + v + "/"
	}
	key = strings.TrimSuffix(key, "/")

	return KitesPrefix + key, nil
}

// getQueryKey returns the etcd key for the query.
func getQueryKey(q *protocol.KontrolQuery) (string, error) {
	fields := map[string]string{
		"username":    q.Username,
		"environment": q.Environment,
		"name":        q.Name,
		"version":     q.Version,
		"region":      q.Region,
		"hostname":    q.Hostname,
		"id":          q.ID,
	}

	if q.Username == "" {
		return "", errors.New("Empty username field")
	}

	// Validate query and build key.
	path := "/"

	empty := false   // encountered with empty field?
	empytField := "" // for error log
	for k, v := range fields {
		if v == "" {
			empty = true
			empytField = k
			continue
		}

		if empty && v != "" {
			return "", fmt.Errorf("Invalid query. Query option is not set: %s", empytField)
		}

		path = path + v + "/"
	}

	path = strings.TrimSuffix(path, "/")

	return KitesPrefix + path, nil
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	args := r.Args.MustSlice()

	if len(args) != 1 && len(args) != 2 {
		return nil, errors.New("Invalid number of arguments")
	}

	var query protocol.KontrolQuery
	err := args[0].Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query argument")
	}

	// To be called when a Kite is registered or deregistered matching the query.
	var watchCallback dnode.Function
	if len(args) == 2 {
		watchCallback = args[1].MustFunction()
	}

	// We do not allow access to other's kites for now.
	if r.Username != query.Username {
		return nil, errors.New("Not your Kite")
	}

	return k.getKites(r, query, watchCallback)
}

func (k *Kontrol) getKites(r *kite.Request, query protocol.KontrolQuery, watchCallback dnode.Function) ([]*protocol.KiteWithToken, error) {
	key, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.GetAll(key, false)
	if err != nil {
		if etcdErr, ok := err.(etcd.EtcdError); ok {
			if etcdErr.ErrorCode == 100 { // Key Not Found
				return make([]*protocol.KiteWithToken, 0), nil
			}
		}
		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("Internal error")
	}

	kvs := flatten(resp.Kvs)

	kitesWithToken, err := addTokenToKites(kvs, r.Username)
	if err != nil {
		return nil, err
	}

	// Register callbacks to our watcher hub.
	// It will call them when a Kite registered/unregistered matching the query.
	if watchCallback != nil {
		k.watcherHub.RegisterWatcher(r.RemoteKite, &query, watchCallback)
	}

	return kitesWithToken, nil
}

// flatten converts the recursive etcd directory structure to flat one that contains Kites.
func flatten(in []etcd.KeyValuePair) []etcd.KeyValuePair {
	var out []etcd.KeyValuePair

	for _, kv := range in {
		if kv.Dir {
			out = append(out, flatten(kv.KVPairs)...)
			continue
		}

		out = append(out, kv)
	}

	return out
}

func addTokenToKites(kvs []etcd.KeyValuePair, username string) ([]*protocol.KiteWithToken, error) {
	kitesWithToken := make([]*protocol.KiteWithToken, len(kvs))

	for i, kv := range kvs {
		kite, kodingKey, err := kiteFromEtcdKV(kv.Key, kv.Value)
		if err != nil {
			return nil, err
		}

		kitesWithToken[i], err = addTokenToKite(kite, username, kodingKey)
		if err != nil {
			return nil, err
		}
	}

	return kitesWithToken, nil
}

func addTokenToKite(kite *protocol.Kite, username, kodingKey string) (*protocol.KiteWithToken, error) {
	token, err := generateToken(kite, username, kodingKey)
	if err != nil {
		return nil, err
	}

	return &protocol.KiteWithToken{
		Kite:  *kite,
		Token: token,
	}, nil
}

func generateToken(kite *protocol.Kite, username, kodingKey string) (string, error) {
	key, err := kodingkey.FromString(kodingKey)
	if err != nil {
		return "", fmt.Errorf("Koding Key is invalid at Kite: %s", key)
	}

	// username is from requester, key is from kite owner.
	return token.NewToken(username, kite.ID).EncryptString(key)
}

// kiteFromEtcdKV returns a *protocol.Kite and Koding Key string from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKV(key, value string) (*protocol.Kite, string, error) {
	fields := strings.Split(strings.TrimPrefix(key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, "", fmt.Errorf("Invalid Kite: %s", key)
	}

	kite := new(protocol.Kite)
	kite.Username = fields[1]
	kite.Environment = fields[2]
	kite.Name = fields[3]
	kite.Version = fields[4]
	kite.Region = fields[5]
	kite.Hostname = fields[6]
	kite.ID = fields[7]

	rv := new(registerValue)
	json.Unmarshal([]byte(value), rv)

	kite.PublicIP = rv.PublicIP
	kite.Port = rv.Port

	return kite, rv.KodingKey, nil
}

// WatchEtcd watches all Kite changes on etcd cluster
// and notifies registered watchers on this Kontrol instance.
func (k *Kontrol) WatchEtcd() {
	var resp *etcd.Response
	var err error

	for {
		resp, err = k.etcd.Set("/_kontrol_get_index", "OK", 1)
		if err == nil {
			break
		}

		log.Critical("etcd error 1: %s", err.Error())
		time.Sleep(time.Second)
	}

	index := resp.ModifiedIndex
	log.Info("etcd: index = %d", index)

	receiver := make(chan *etcd.Response)

	go func() {
		for {
			_, err := k.etcd.WatchAll(KitesPrefix, index+1, receiver, nil)
			if err != nil {
				log.Critical("etcd error 2: %s", err)
				time.Sleep(time.Second)
			}
		}
	}()

	// Channel is never closed.
	for resp := range receiver {
		// log.Debug("etcd: change received: %#v", resp)
		index = resp.ModifiedIndex

		// Notify deregistration events.
		if strings.HasPrefix(resp.Key, KitesPrefix) && (resp.Action == "delete" || resp.Action == "expire") {
			kite, _, err := kiteFromEtcdKV(resp.Key, resp.Value)
			if err == nil {
				k.watcherHub.Notify(kite, protocol.Deregister, "")
			}
		}
	}
}

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var kite *protocol.Kite
	err := r.Args.Unmarshal(&kite)
	if err != nil {
		return nil, errors.New("Invalid Kite")
	}

	kiteKey, err := getKiteKey(kite)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.Get(kiteKey, false)
	if err != nil {
		return nil, err
	}

	var kiteVal registerValue
	err = json.Unmarshal([]byte(resp.Value), &kiteVal)
	if err != nil {
		return nil, err
	}

	return generateToken(kite, r.Username, kiteVal.KodingKey)
}

func (k *Kontrol) AuthenticateFromSessionID(options *kite.CallOptions) error {
	username, err := findUsernameFromSessionID(options.Authentication.Key)
	if err != nil {
		return err
	}

	options.Kite.Username = username

	return nil
}

func findUsernameFromSessionID(sessionID string) (string, error) {
	session, err := modelhelper.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.Username, nil
}

func (k *Kontrol) AuthenticateFromKodingKey(options *kite.CallOptions) error {
	username, err := findUsernameFromKey(options.Authentication.Key)
	if err != nil {
		return err
	}

	options.Kite.Username = username

	return nil
}

func findUsernameFromKey(key string) (string, error) {
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
