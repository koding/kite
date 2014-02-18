// Package kontrol provides an implementation for the name service kite.
// It can be queried to get the list of running kites.
package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"kite"
	"kite/dnode"
	"kite/logging"
	"kite/protocol"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/store"
	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatDelay    = 10 * time.Second
	KitesPrefix       = "/kites"
	TokenTTL          = 1 * time.Hour
	TokenLeeway       = 1 * time.Minute
)

var log logging.Logger

type Kontrol struct {
	kite       *kite.Kite
	ip         string
	port       int
	name       string
	dataDir    string
	peers      []string
	store      store.Store
	psListener net.Listener
	sListener  net.Listener
	publicKey  string
	privateKey string
}

// New creates a new kontrol.
//
// peers can be given nil if not running on cluster.
//
// Public and private keys are RSA pem blocks that can be generated with the
// following command:
//     openssl genrsa -out testkey.pem 2048
//     openssl rsa -in testkey.pem -pubout > testkey_pub.pem
//
func New(kiteOptions *kite.Options, name, dataDir string, peers []string, publicKey, privateKey string) *Kontrol {
	port, err := strconv.Atoi(kiteOptions.Port)
	if err != nil {
		panic(err.Error())
	}

	kontrol := &Kontrol{
		kite:       kite.New(kiteOptions),
		ip:         kiteOptions.PublicIP,
		port:       port,
		name:       name,
		dataDir:    dataDir,
		peers:      peers,
		publicKey:  publicKey,
		privateKey: privateKey,
	}

	log = kontrol.kite.Log

	kontrol.kite.KontrolEnabled = false // Because we are Kontrol!

	kontrol.kite.HandleFunc("register", kontrol.handleRegister)
	kontrol.kite.HandleFunc("getKites", kontrol.handleGetKites)
	kontrol.kite.HandleFunc("getToken", kontrol.handleGetToken)

	return kontrol
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.kite.Authenticators[keyType] = fn
}

func (k *Kontrol) EnableTLS(certFile, keyFile string) {
	k.kite.EnableTLS(certFile, keyFile)
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
	rand.Seed(time.Now().UnixNano())

	// Run etcd
	etcdReady := make(chan bool)
	go k.runEtcd(etcdReady)
	<-etcdReady

	go k.registerSelf()
}

// ClearKites removes everything under "/kites" from etcd.
func (k *Kontrol) ClearKites() error {
	_, err := k.store.Delete(KitesPrefix, true, true)
	if err != nil && err.(*etcdErr.Error).ErrorCode != etcdErr.EcodeKeyNotFound {
		return fmt.Errorf("Cannot clear etcd: %s", err)
	}
	return nil
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	URL protocol.KiteURL
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %#v", r.RemoteKite.Kite)

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	if r.RemoteKite.URL == nil {
		return nil, errors.New("Empty 'url' field")
	}

	// In case Kite.URL does not contain a hostname, the r.RemoteAddr is used.
	host, port, _ := net.SplitHostPort(r.RemoteKite.URL.Host)
	if host == "0.0.0.0" {
		host, _, _ = net.SplitHostPort(r.RemoteAddr)
		r.RemoteKite.URL.Host = net.JoinHostPort(host, port)
	}

	return k.register(r.RemoteKite, r.RemoteAddr)
}

func (k *Kontrol) register(r *kite.RemoteKite, remoteAddr string) (*protocol.RegisterResult, error) {
	// Need a copy of protocol.Kite structure because URL field is overwritten
	// by the heartbeat request (in request.go:parseRequest).
	var kiteCopy protocol.Kite = r.Kite
	kite := &kiteCopy // shorthand

	kiteKey, err := getKiteKey(kite)
	if err != nil {
		return nil, err
	}

	// setKey sets the value of the Kite in etcd.
	setKey := k.makeSetter(kite, kiteKey)

	// Register to etcd.
	err = setKey()
	if err != nil {
		log.Critical("etcd setKey error: %s", err)
		return nil, errors.New("internal error - register")
	}

	if err := requestHeartbeat(r, setKey); err != nil {
		return nil, err
	}

	log.Info("Kite registered: %s", kiteKey)

	r.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		k.store.Delete(kiteKey, false, false)
	})

	// send response back to the kite, also identify him with the new name
	ip, _, _ := net.SplitHostPort(remoteAddr)
	return &protocol.RegisterResult{PublicIP: ip}, nil
}

func requestHeartbeat(r *kite.RemoteKite, setterFunc func() error) error {
	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		kite.Callback(func(r *kite.Request) { setterFunc() }),
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

	setter := k.makeSetter(&k.kite.Kite, key)
	for {
		if err := setter(); err != nil {
			log.Error(err.Error())
			time.Sleep(time.Second)
			continue
		}

		time.Sleep(HeartbeatInterval)
	}
}

//  makeSetter returns a func for setting the kite key with value in etcd.
func (k *Kontrol) makeSetter(kite *protocol.Kite, etcdKey string) func() error {
	rv := &registerValue{
		URL: *kite.URL,
	}

	valueBytes, _ := json.Marshal(rv)
	value := string(valueBytes)

	return func() error {
		expireAt := time.Now().Add(HeartbeatDelay)

		// Set the kite
		_, err := k.store.Set(etcdKey, false, value, expireAt)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return err
		}

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		_, err = k.store.Update(KitesPrefix+"/"+kite.Username, "", expireAt)
		if err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		return nil
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

	return path, nil
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
		if canAccess(r.RemoteKite.Kite, query) {
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

	// Register callbacks to our watcher hub.
	// It will call them when a Kite registered/unregistered matching the query.
	// Regsitering watcher should be done before making etcd.Get().
	if watchCallback != nil {
		watcher, err := k.store.Watch(KitesPrefix+key, true, true, 0)
		if err != nil {
			return nil, err
		}

		// Stop watching on disconnect.
		disconnect := make(chan bool)
		r.RemoteKite.OnDisconnect(func() {
			close(disconnect)
			watcher.Remove()
		})

		go k.watchAndSendKiteEvents(watcher, disconnect, &query, watchCallback)
	}

	// Get kites from etcd
	event, err := k.store.Get(
		KitesPrefix+key,
		true,  // recursive, return all child directories too
		false, // sorting flag, we don't care about sorting for now
	)
	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			return make([]*protocol.KiteWithToken, 0), nil
		}

		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("internal error - getKites")
	}

	// Attach tokens to kites
	kitesAndTokens, err := addTokenToKites(flatten(event.Node.Nodes), r.Username, k.kite.Username, key, k.privateKey)
	if err != nil {
		return nil, err
	}

	// Shuffle the list
	shuffled := make([]*protocol.KiteWithToken, len(kitesAndTokens))
	perm := rand.Perm(len(kitesAndTokens))
	for i, v := range perm {
		shuffled[v] = kitesAndTokens[i]
	}

	return shuffled, nil
}

func (k *Kontrol) watchAndSendKiteEvents(watcher *store.Watcher, disconnect chan bool, query *protocol.KontrolQuery, callback dnode.Function) {
	var index uint64 = 0
	for {
		var kiteEvent kite.Event

		select {
		case <-disconnect:
			return
		case etcdEvent, ok := <-watcher.EventChan:
			index = etcdEvent.Node.ModifiedIndex

			// If EventChan is closed then eiter we couldn't consume
			// messages on time or the remote kite is disconnected
			// and the watcher is removed.
			if !ok {
				// Do not burn CPU until OnDisconnect handler is called
				// if the reason of cancel is disconnect.
				time.Sleep(time.Second)

				// If the watcher is cancelled because we don't consume at
				// enough rate, then we are going to try to re-watch the same key.
				key, _ := getQueryKey(query) // can't fail
				var err error
				watcher, err = k.store.Watch(KitesPrefix+key, true, true, index)
				if err != nil {
					log.Warning("Cannot re-watch query: %s", err.Error())
					return // TODO find a way to tell the error to the kite
				}

				continue
			}

			switch etcdEvent.Action {
			case store.Set:
				// Do not send Register events for heartbeat messages.
				// PrevNode must be empty if the kite has registered for the first time.
				if etcdEvent.PrevNode != nil {
					continue
				}

				otherKite, err := kiteFromEtcdKV(etcdEvent.Node.Key, etcdEvent.Node.Value)
				if err != nil {
					continue
				}

				kiteEvent.Action = protocol.Register
				kiteEvent.Kite = *otherKite

				kiteEvent.Token, err = generateToken(etcdEvent.Node.Key, query.Username, k.kite.Username, k.privateKey)
				if err != nil {
					log.Error("watch notify: %s", err)
					return
				}
			case store.Delete: // Delete happens when we detect that otherKite is disconnected.
				fallthrough
			case store.Expire: // Expire happens when we don't get heartbeat from otherKite.
				otherKite, err := kiteFromEtcdKV(etcdEvent.Node.Key, etcdEvent.Node.Value)
				if err != nil {
					continue
				}

				kiteEvent.Action = protocol.Deregister
				kiteEvent.Kite = *otherKite
			default:
				continue // We don't care other events
			}

			callback(kiteEvent)
		}
	}
}

// flatten converts the recursive etcd directory structure to flat one that contains Kites.
func flatten(in store.NodeExterns) (out store.NodeExterns) {
	for _, node := range in {
		if node.Dir {
			out = append(out, flatten(node.Nodes)...)
			continue
		}

		out = append(out, node)
	}

	return
}

func addTokenToKites(nodes store.NodeExterns, username, issuer, queryKey, privateKey string) ([]*protocol.KiteWithToken, error) {
	kitesWithToken := make([]*protocol.KiteWithToken, len(nodes))

	for i, node := range nodes {
		kite, err := kiteFromEtcdKV(node.Key, node.Value)
		if err != nil {
			return nil, err
		}

		kitesWithToken[i], err = addTokenToKite(kite, username, issuer, queryKey, privateKey)
		if err != nil {
			return nil, err
		}
	}

	return kitesWithToken, nil
}

func addTokenToKite(kite *protocol.Kite, username, issuer, queryKey, privateKey string) (*protocol.KiteWithToken, error) {
	tkn, err := generateToken(queryKey, username, issuer, privateKey)
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
func generateToken(queryKey string, username, issuer, privateKey string) (string, error) {
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	// Identifies the expiration time after which the JWT MUST NOT be accepted
	// for processing.
	ttl := TokenTTL

	// Implementers MAY provide for some small leeway, usually no more than
	// a few minutes, to account for clock skew.
	leeway := TokenLeeway

	tkn := jwt.New(jwt.GetSigningMethod("RS256"))
	tkn.Claims["iss"] = issuer                                       // Issuer
	tkn.Claims["sub"] = username                                     // Subject
	tkn.Claims["aud"] = queryKey                                     // Audience
	tkn.Claims["exp"] = time.Now().UTC().Add(ttl).Add(leeway).Unix() // Expiration Time
	tkn.Claims["nbf"] = time.Now().UTC().Add(-leeway).Unix()         // Not Before
	tkn.Claims["iat"] = time.Now().UTC().Unix()                      // Issued At
	tkn.Claims["jti"] = tknID.String()                               // JWT ID

	signed, err := tkn.SignedString([]byte(privateKey))
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	return signed, nil
}

// kiteFromEtcdKV returns a *protocol.Kite and Koding Key string from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKV(key, value string) (*protocol.Kite, error) {
	fields := strings.Split(strings.TrimPrefix(key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, fmt.Errorf("Invalid Kite: %s", key)
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

	kite.URL = &rv.URL

	return kite, nil
}

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var query protocol.KontrolQuery
	err := r.Args.One().Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query")
	}

	if !canAccess(r.RemoteKite.Kite, query) {
		return nil, errors.New("Forbidden")
	}

	kiteKey, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	event, err := k.store.Get(KitesPrefix+kiteKey, false, false)
	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			return nil, errors.New("Kite not found")
		}
		return nil, err
	}

	var kiteVal registerValue
	err = json.Unmarshal([]byte(event.Node.Value), &kiteVal)
	if err != nil {
		return nil, err
	}

	return generateToken(kiteKey, r.Username, k.kite.Username, k.privateKey)
}

// canAccess makes some access control checks and returns true
// if k can access to kites matching the query.
func canAccess(k protocol.Kite, query protocol.KontrolQuery) bool {
	return true
}
