package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"koding/db/mongodb/modelhelper"
	"koding/kite/dnode"
	"koding/kite"
	"koding/kite/protocol"
	"koding/tools/config"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
	"github.com/op/go-logging"
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
		Version:     "0.0.1",
		Port:        strconv.Itoa(config.Current.NewKontrol.Port),
		Region:      "sj",
		Environment: "development",
		Username:    "koding",
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

	// Disable until we got all things set up - arslan
	// kontrol.kite.EnableTLS(
	// 	config.Current.NewKontrol.CertFile,
	// 	config.Current.NewKontrol.KeyFile,
	// )

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
	go k.registerSelf()
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	URL        protocol.KiteURL
	KodingKey  string
	Visibility protocol.Visibility
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %#v", r.RemoteKite.Kite)

	// Only accept requests with kodingKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kodingKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	if r.RemoteKite.URL.URL == nil {
		return nil, errors.New("Empty 'url' field")
	}

	// In case Kite.URL does not contain a hostname, the r.RemoteAddr is used.
	host, port, _ := net.SplitHostPort(r.RemoteKite.URL.Host)
	if host == "" {
		host, _, _ = net.SplitHostPort(r.RemoteAddr)
		r.RemoteKite.URL.Host = net.JoinHostPort(host, port)
	}

	return k.register(r.RemoteKite, r.Authentication.Key, r.RemoteAddr)
}

func (k *Kontrol) register(r *kite.RemoteKite, kodingkey, remoteAddr string) (*protocol.RegisterResult, error) {
	kite := &r.Kite

	if kite.Visibility != protocol.Public && kite.Visibility != protocol.Private {
		return nil, errors.New("Invalid visibility field")
	}

	key, err := getKiteKey(kite)
	if err != nil {
		return nil, err
	}

	// setKey sets the value of the Kite in etcd.
	setKey := k.makeSetter(kite, key, kodingkey)

	// Register to etcd.
	prev, err := setKey()
	if err != nil {
		log.Critical("etcd setKey error: %s", err)
		return nil, errors.New("internal error - register")
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
		k.etcd.Delete(key, false)
	})

	// send response back to the kite, also identify him with the new name
	ip, _, _ := net.SplitHostPort(remoteAddr)
	return &protocol.RegisterResult{
		Result:   protocol.AllowKite,
		Username: r.Username,
		PublicIP: ip,
	}, nil
}

func requestHeartbeat(r *kite.RemoteKite, setterFunc func() (string, error)) error {
	heartbeatFunc := func(req *kite.Request) {
		prev, err := setterFunc()
		if err == nil && prev == "" {
			log.Warning("Came heartbeat but the Kite (%s) is not registered. Re-registering it. It may be an indication that the heartbeat delay is too short.", req.RemoteKite.ID)
		}
	}

	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		kite.Callback(heartbeatFunc),
	}

	_, err := r.Tell("heartbeat", heartbeatArgs...)
	return err
}

// registerSelf adds Kontrol itself to etcd.
func (k *Kontrol) registerSelf() {
	key, err := getKiteKey(&k.kite.Kite)
	if err != nil {
		panic(err)
	}

	setter := k.makeSetter(&k.kite.Kite, key, k.kite.KodingKey)
	for {
		if _, err := setter(); err != nil {
			log.Critical(err.Error())
			time.Sleep(time.Second)
			continue
		}

		time.Sleep(HeartbeatInterval)
	}
}

//  makeSetter returns a func for setting the kite key with value in etcd.
func (k *Kontrol) makeSetter(kite *protocol.Kite, etcdKey, kodingkey string) func() (string, error) {
	rv := &registerValue{
		URL:        kite.URL,
		KodingKey:  kodingkey,
		Visibility: kite.Visibility,
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

		if resp.PrevNode != nil {
			prevValue = resp.PrevNode.Value
		}

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
	if len(r.Args) != 1 && len(r.Args) != 2 {
		return nil, errors.New("Invalid number of arguments")
	}

	var query protocol.KontrolQuery
	err := r.Args[0].Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query argument")
	}

	// To be called when a Kite is registered or deregistered matching the query.
	var watchCallback dnode.Function
	if len(r.Args) == 2 {
		watchCallback = r.Args[1].MustFunction()
	}

	kites, err := k.getKites(r, query, watchCallback)
	if err != nil {
		return nil, err
	}

	allowed := make([]*protocol.KiteWithToken, 0, len(kites))
	for _, kite := range kites {
		if canAccess(r.RemoteKite.Kite, kite.Kite) {
			allowed = append(allowed, kite)
		}
	}

	return allowed, nil
}

func (k *Kontrol) getKites(r *kite.Request, query protocol.KontrolQuery, watchCallback dnode.Function) ([]*protocol.KiteWithToken, error) {
	key, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.Get(
		key,
		false, // sorting flag, we don't care about sorting for now
		true,  // recursive, return all child directories too
	)
	if err != nil {
		if etcdErr, ok := err.(*etcd.EtcdError); ok {
			if etcdErr.ErrorCode == 100 { // Key Not Found
				return make([]*protocol.KiteWithToken, 0), nil
			}
		}

		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("internal error - getKites")
	}

	kvs := flatten(resp.Node.Nodes)

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
func flatten(in etcd.Nodes) (out etcd.Nodes) {
	for _, node := range in {
		if node.Dir {
			out = append(out, flatten(node.Nodes)...)
			continue
		}

		out = append(out, node)
	}

	return
}

func addTokenToKites(nodes etcd.Nodes, username string) ([]*protocol.KiteWithToken, error) {
	kitesWithToken := make([]*protocol.KiteWithToken, len(nodes))

	for i, node := range nodes {
		kite, kodingKey, err := kiteFromEtcdKV(node.Key, node.Value)
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
	tkn, err := generateToken(kite, username)
	if err != nil {
		return nil, err
	}

	return &protocol.KiteWithToken{
		Kite:  *kite,
		Token: tkn,
	}, nil
}

// generateToken returns a JWT token string. Please see the URL for details:
// http://tools.ietf.org/html/draft-ietf-oauth-json-web-token-13#section-4.1
func generateToken(kite *protocol.Kite, username string) (string, error) {
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	// Identifies the expiration time after which the JWT MUST NOT be accepted
	// for processing.
	ttl := 1 * time.Hour

	// Implementers MAY provide for some small leeway, usually no more than
	// a few minutes, to account for clock skew.
	leeway := 1 * time.Minute

	tkn := jwt.New(jwt.GetSigningMethod("RS256"))
	tkn.Claims["iss"] = "koding.com"                                 // Issuer
	tkn.Claims["sub"] = username                                     // Subject
	tkn.Claims["aud"] = kite.ID                                      // Audience
	tkn.Claims["exp"] = time.Now().UTC().Add(ttl).Add(leeway).Unix() // Expiration Time
	tkn.Claims["nbf"] = time.Now().UTC().Add(-leeway).Unix()         // Not Before
	tkn.Claims["iat"] = time.Now().UTC().Unix()                      // Issued At
	tkn.Claims["jti"] = tknID.String()                               // JWT ID

	signed, err := tkn.SignedString(rsaKey)
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	return signed, nil
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

	kite.URL = rv.URL

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

	index := resp.Node.ModifiedIndex
	log.Info("etcd: index = %d", index)

	receiver := make(chan *etcd.Response)

	go func() {
		for {
			_, err := k.etcd.Watch(KitesPrefix, index+1, true, receiver, nil)
			if err != nil {
				log.Critical("etcd error 2: %s", err)
				time.Sleep(time.Second)
			}
		}
	}()

	// Channel is never closed.
	for resp := range receiver {
		// log.Debug("etcd: change received: %#v", resp)
		index = resp.Node.ModifiedIndex

		// Notify deregistration events.
		if strings.HasPrefix(resp.Node.Key, KitesPrefix) && (resp.Action == "delete" || resp.Action == "expire") {
			kite, _, err := kiteFromEtcdKV(resp.Node.Key, resp.Node.Value)
			if err == nil {
				k.watcherHub.Notify(kite, protocol.Deregister, "")
			}
		}
	}
}

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var kite *protocol.Kite
	err := r.Args.MustSliceOfLength(1)[0].Unmarshal(&kite)
	if err != nil {
		return nil, errors.New("Invalid Kite")
	}

	if !canAccess(r.RemoteKite.Kite, *kite) {
		return nil, errors.New("Forbidden")
	}

	kiteKey, err := getKiteKey(kite)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.Get(kiteKey, false, false)
	if err != nil {
		return nil, err
	}

	var kiteVal registerValue
	err = json.Unmarshal([]byte(resp.Node.Value), &kiteVal)
	if err != nil {
		return nil, err
	}

	return generateToken(kite, r.Username)
}

// canAccess makes some access control checks and returns true
// if fromKite can talk with toKite.
func canAccess(fromKite protocol.Kite, toKite protocol.Kite) bool {
	// Do not allow other users if kite is private.
	if fromKite.Username != toKite.Username && toKite.Visibility == protocol.Private {
		return false
	}

	// Prevent access to development/staging kites if the requester is not owner.
	if fromKite.Username != toKite.Username && toKite.Environment != "production" {
		return false
	}

	return true
}

func (k *Kontrol) AuthenticateFromSessionID(r *kite.Request) error {
	username, err := findUsernameFromSessionID(r.Authentication.Key)
	if err != nil {
		return err
	}

	r.Username = username

	return nil
}

func findUsernameFromSessionID(sessionID string) (string, error) {
	session, err := modelhelper.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.Username, nil
}

func (k *Kontrol) AuthenticateFromKodingKey(r *kite.Request) error {
	username, err := findUsernameFromKey(r.Authentication.Key)
	if err != nil {
		return err
	}

	r.Username = username

	return nil
}

func findUsernameFromKey(key string) (string, error) {
	kodingKey, err := modelhelper.GetKodingKeysByKey(key)
	if err != nil {
		return "", errors.New("kodingkey not found in kontrol db")
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
