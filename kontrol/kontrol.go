// Package kontrol provides an implementation for the name service kite.
// It can be queried to get the list of running kites.
package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/store"
	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/server"
	"github.com/koding/logging"
	"github.com/nu7hatch/gouuid"
)

const (
	Version           = "0.0.2"
	DefaultPort       = 4000
	HeartbeatInterval = 5 * time.Second
	HeartbeatDelay    = 10 * time.Second
	KitesPrefix       = "/kites"
	TokenTTL          = 1 * time.Hour
	TokenLeeway       = 1 * time.Minute
)

var log logging.Logger

type Kontrol struct {
	Server       *server.Server
	Name         string       // Name of the etcd instance
	DataDir      string       // etcd data dir
	EtcdAddr     string       // The public host:port used for etcd server.
	EtcdBindAddr string       // The listening host:port used for etcd server.
	PeerAddr     string       // The public host:port used for peer communication.
	PeerBindAddr string       // The listening host:port used for peer communication.
	Peers        []string     // other peers in cluster (must be peer address of other instances)
	store        store.Store  // etcd data store
	psListener   net.Listener // etcd peer server listener (default port: 7001)
	sListener    net.Listener // etcd http server listener (default port: 4001)
	publicKey    string       // RSA key for validation of tokens
	privateKey   string       // RSA key for signing tokens

	// To cancel running watchers, we must store the references
	watchers      map[string]*store.Watcher
	watchersMutex sync.Mutex
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
func New(conf *config.Config, publicKey, privateKey string) *Kontrol {
	k := kite.New("kontrol", Version)
	k.Config = conf

	// Listen on 4000 by default
	if k.Config.Port == 0 {
		k.Config.Port = DefaultPort
	}

	hostname := k.Kite().Hostname

	kontrol := &Kontrol{
		Server:       server.New(k),
		Name:         hostname,
		DataDir:      "kontrol-data." + hostname,
		EtcdAddr:     "http://localhost:4001",
		EtcdBindAddr: ":4001",
		PeerAddr:     "http://localhost:7001",
		PeerBindAddr: ":7001",
		Peers:        nil,
		publicKey:    publicKey,
		privateKey:   privateKey,
		watchers:     make(map[string]*store.Watcher),
	}

	log = k.Log

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("getKites", kontrol.handleGetKites)
	k.HandleFunc("getToken", kontrol.handleGetToken)
	k.HandleFunc("cancelWatcher", kontrol.handleCancelWatcher)

	return kontrol
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.Server.Kite.Authenticators[keyType] = fn
}

// func (k *Kontrol) EnableTLS(certFile, keyFile string) {
// 	k.Server.Kite.EnableTLS(certFile, keyFile)
// }

func (k *Kontrol) Run() {
	k.init()
	k.Server.Run()
}

func (k *Kontrol) Start() {
	k.init()
	k.Server.Start()
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
	_, err := k.store.Delete(
		KitesPrefix, // path
		true,        // recursive
		true,        // dir
	)
	if err != nil && err.(*etcdErr.Error).ErrorCode != etcdErr.EcodeKeyNotFound {
		return fmt.Errorf("Cannot clear etcd: %s", err)
	}
	return nil
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	URL *protocol.KiteURL `json:"url"`
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %s", r.Client.Kite)

	if r.Args.One().MustMap()["url"].MustString() == "" {
		return nil, errors.New("invalid url")
	}

	var args struct {
		URL *protocol.KiteURL `json:"url"`
	}
	r.Args.One().MustUnmarshal(&args)
	if args.URL == nil {
		return nil, errors.New("empty url")
	}

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	// In case Kite.URL does not contain a hostname, the r.RemoteAddr is used.
	host, port, _ := net.SplitHostPort(args.URL.Host)
	if host == "0.0.0.0" || host == "" {
		host, _, _ = net.SplitHostPort(r.RemoteAddr)
		args.URL.Host = net.JoinHostPort(host, port)
	}

	err := k.register(r.Client, args.URL)
	if err != nil {
		return nil, err
	}

	// send response back to the kite, also identify him with the new name
	return &protocol.RegisterResult{URL: args.URL.String()}, nil
}

func (k *Kontrol) register(r *kite.Client, kiteURL *protocol.KiteURL) error {
	err := validateKiteKey(&r.Kite)
	if err != nil {
		return err
	}

	value := &registerValue{
		URL: kiteURL,
	}

	// setKey sets the value of the Kite in etcd.
	setKey, etcdKey := k.makeSetter(&r.Kite, value)

	// Register to etcd.
	err = setKey()
	if err != nil {
		log.Critical("etcd setKey error: %s", err)
		return errors.New("internal error - register")
	}

	if err := requestHeartbeat(r, setKey); err != nil {
		return err
	}

	log.Info("Kite registered: %s", r.Kite)

	r.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		k.store.Delete(
			etcdKey, // path
			false,   // recursive
			false,   // dir
		)
	})

	return nil
}

func requestHeartbeat(r *kite.Client, setterFunc func() error) error {
	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		kite.Callback(func(args dnode.Arguments) { setterFunc() }),
	}

	_, err := r.Tell("kite.heartbeat", heartbeatArgs...)
	return err
}

// registerSelf adds Kontrol itself to etcd as a kite.
func (k *Kontrol) registerSelf() {
	value := &registerValue{
		URL: &protocol.KiteURL{*k.Server.Config.KontrolURL},
	}
	setter, _ := k.makeSetter(k.Server.Kite.Kite(), value)
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
func (k *Kontrol) makeSetter(kite *protocol.Kite, value *registerValue) (setter func() error, etcdKey string) {
	etcdKey = KitesPrefix + kite.String()

	valueBytes, _ := json.Marshal(value)
	valueString := string(valueBytes)

	setter = func() error {
		expireAt := time.Now().Add(HeartbeatDelay)

		// Set the kite key.
		// Example "/koding/production/os/0.0.1/sj/kontainer1.sj.koding.com/1234asdf..."
		_, err := k.store.Set(
			etcdKey,     // path
			false,       // dir
			valueString, // value
			expireAt,    // expire time
		)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return err
		}

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		_, err = k.store.Update(
			KitesPrefix+"/"+kite.Username, // path
			"",       // new value
			expireAt, // expire time
		)
		if err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		return nil
	}

	return
}

// validateKiteKey returns a string representing the kite uniquely
// that is suitable to use as a key for etcd.
func validateKiteKey(k *protocol.Kite) error {
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
			return fmt.Errorf("Empty Kite field: %s", k)
		}
		if strings.ContainsRune(v, '/') {
			return fmt.Errorf("Field \"%s\" must not contain '/'", k)
		}
	}

	return nil
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

	return k.getKites(r, query, watchCallback)
}

func (k *Kontrol) getKites(r *kite.Request, query protocol.KontrolQuery, watchCallback dnode.Function) (*protocol.GetKitesResult, error) {
	key, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	var result = new(protocol.GetKitesResult)

	// Create e watcher on query.
	// The callback is going to be called when a Kite registered/unregistered
	// matching the query.
	// Registering watcher should be done before making etcd.Get() because
	// Get() may return an empty result.
	if watchCallback != nil {
		watcher, err := k.store.Watch(
			KitesPrefix+key, // prefix
			true,            // recursive
			true,            // stream
			0,               // since index
		)
		if err != nil {
			return nil, err
		}

		watcherID, err := uuid.NewV4()
		if err != nil {
			return nil, err
		}
		result.WatcherID = watcherID.String()

		// Put watcher into map in order to cancel from cancelWatcher() method.
		k.watchersMutex.Lock()
		k.watchers[result.WatcherID] = watcher

		// Stop watching on disconnect.
		disconnect := make(chan bool)
		r.Client.OnDisconnect(func() {
			// Remove watcher from the map
			k.watchersMutex.Lock()
			defer k.watchersMutex.Unlock()
			delete(k.watchers, watcherID.String())

			// Notify disconnection and stop watching.
			close(disconnect)
			watcher.Remove()
		})
		k.watchersMutex.Unlock()

		go k.watchAndSendKiteEvents(watcher, result.WatcherID, disconnect, &query, watchCallback)
	}

	// Get kites from etcd
	event, err := k.store.Get(
		KitesPrefix+key, // path
		true,            // recursive, return all child directories too
		false,           // sorting flag, we don't care about sorting for now
	)
	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			result.Kites = make([]*protocol.KiteWithToken, 0) // do not send null
			return result, nil
		}

		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("internal error - getKites")
	}

	// Attach tokens to kites
	kitesAndTokens, err := addTokenToKites(flatten(event.Node.Nodes), r.Username, k.Server.Kite.Kite().Username, key, k.privateKey)
	if err != nil {
		return nil, err
	}

	// Shuffle the list
	shuffled := make([]*protocol.KiteWithToken, len(kitesAndTokens))
	perm := rand.Perm(len(kitesAndTokens))
	for i, v := range perm {
		shuffled[v] = kitesAndTokens[i]
	}

	result.Kites = shuffled
	return result, nil
}

func (k *Kontrol) handleCancelWatcher(r *kite.Request) (interface{}, error) {
	id := r.Args.One().MustString()
	return nil, k.cancelWatcher(id)
}

func (k *Kontrol) cancelWatcher(watcherID string) error {
	k.watchersMutex.Lock()
	defer k.watchersMutex.Unlock()
	watcher, ok := k.watchers[watcherID]
	if !ok {
		return errors.New("Watcher not found")
	}
	watcher.Remove()
	delete(k.watchers, watcherID)
	return nil
}

func (k *Kontrol) watchAndSendKiteEvents(watcher *store.Watcher, watcherID string, disconnect chan bool, query *protocol.KontrolQuery, callback dnode.Function) {
	var index uint64 = 0
	for {
		var response kite.Response

		select {
		case <-disconnect:
			return
		case etcdEvent, ok := <-watcher.EventChan:
			// Channel is closed. This happens in 3 cases:
			//   1. Remote kite called "cancelWatcher" method and removed the watcher.
			//   2. Remote kite has disconnected and the watcher is removed.
			//   3. Remote kite couldn't consume messages fast enough, buffer has filled up and etcd cancelled the watcher.
			if !ok {
				// Do not try again if watcher is cancelled.
				k.watchersMutex.Lock()
				if _, ok := k.watchers[watcherID]; !ok {
					k.watchersMutex.Unlock()
					return
				}

				// Do not try again if disconnected.
				select {
				case <-disconnect:
					k.watchersMutex.Unlock()
					return
				default:
				}
				k.watchersMutex.Unlock()

				// If we are here that means we did not consume fast enough and etcd
				// has canceled our watcher. We need to create a new watcher with the same key.
				key, _ := getQueryKey(query) // can't fail
				var err error
				watcher, err = k.store.Watch(
					KitesPrefix+key, // prefix
					true,            // recursive
					true,            // stream
					index,           // since index
				)
				if err != nil {
					log.Error("Cannot re-watch query: %s", err.Error())
					response.Error = &kite.Error{"watchError", err.Error()}
					callback(response)
					return
				}

				continue
			}

			index = etcdEvent.Node.ModifiedIndex

			switch etcdEvent.Action {
			case store.Set:
				// Do not send Register events for heartbeat messages.
				// PrevNode must be empty if the kite has registered for the first time.
				if etcdEvent.PrevNode != nil {
					continue
				}

				otherKite, err := kiteFromEtcdKV(etcdEvent.Node.Key)
				if err != nil {
					continue
				}

				var val registerValue
				err = json.Unmarshal([]byte(etcdEvent.Node.Value), &val)
				if err != nil {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Register
				e.Kite = *otherKite
				e.URL = val.URL.String()

				e.Token, err = generateToken(etcdEvent.Node.Key, query.Username, k.Server.Kite.Kite().Username, k.privateKey)
				if err != nil {
					log.Error("watch notify: %s", err)
					return
				}

				response.Result = e
			case store.Delete: // Delete happens when we detect that otherKite is disconnected.
				fallthrough
			case store.Expire: // Expire happens when we don't get heartbeat from otherKite.
				otherKite, err := kiteFromEtcdKV(etcdEvent.Node.Key)
				if err != nil {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Deregister
				e.Kite = *otherKite

				response.Result = e
			default:
				continue // We don't care other events
			}

			callback(response)
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
		kite, err := kiteFromEtcdKV(node.Key)
		if err != nil {
			return nil, err
		}

		kitesWithToken[i], err = addTokenToKite(kite, username, issuer, queryKey, privateKey)
		if err != nil {
			return nil, err
		}

		rv := new(registerValue)
		json.Unmarshal([]byte(node.Value), rv)

		kitesWithToken[i].URL = rv.URL.String()
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

// kiteFromEtcdKV returns a *protocol.Kite from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKV(key string) (*protocol.Kite, error) {
	// TODO replace "kites" with KitesPrefix constant
	fields := strings.Split(strings.TrimPrefix(key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, fmt.Errorf("Invalid Kite: %s", key)
	}

	return &protocol.Kite{
		Username:    fields[1],
		Environment: fields[2],
		Name:        fields[3],
		Version:     fields[4],
		Region:      fields[5],
		Hostname:    fields[6],
		ID:          fields[7],
	}, nil
}

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var query protocol.KontrolQuery
	err := r.Args.One().Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query")
	}

	kiteKey, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	event, err := k.store.Get(
		KitesPrefix+kiteKey, // path
		false, // recursive
		false, // sorted
	)
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

	return generateToken(kiteKey, r.Username, k.Server.Kite.Kite().Username, k.privateKey)
}
