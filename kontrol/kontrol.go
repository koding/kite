// Package kontrol provides an implementation for the name service kite.
// It can be queried to get the list of running kites.
package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/etcd"
	"github.com/coreos/etcd/store"
	"github.com/dgrijalva/jwt-go"
	"github.com/hashicorp/go-version"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"github.com/nu7hatch/gouuid"
)

const (
	KontrolVersion    = "0.0.4"
	HeartbeatInterval = 5 * time.Second
	HeartbeatDelay    = 10 * time.Second
	KitesPrefix       = "/kites"
	TokenLeeway       = 1 * time.Minute
)

var (
	log         kite.Logger
	TokenTTL    = 48 * time.Hour
	DefaultPort = 4000

	tokenCache   = make(map[string]string)
	tokenCacheMu sync.Mutex
)

type Kontrol struct {
	Kite *kite.Kite

	// etcd options
	Name         string   // Name of the etcd instance
	DataDir      string   // etcd data dir
	EtcdAddr     string   // The public host:port used for etcd server.
	EtcdBindAddr string   // The listening host:port used for etcd server.
	PeerAddr     string   // The public host:port used for peer communication.
	PeerBindAddr string   // The listening host:port used for peer communication.
	Peers        []string // other peers in cluster (must be peer address of other instances)

	// MachineAuthenticate is used to authenticate the request in the
	// "handleMachine" method.  The reason for a separate auth function is, the
	// request must not be authenticated because clients do not have a kite.key
	// before they register to this machine.
	MachineAuthenticate func(r *kite.Request) error

	// RSA keys
	publicKey  string // for validating tokens
	privateKey string // for signing tokens

	// To cancel running watchers, we must store the references
	watchers      map[string]*store.Watcher
	watchersMutex sync.Mutex

	// Holds refence to all connected clients (key is ID of kite)
	clients map[string]*kite.Client

	etcd *etcd.Etcd
}

// New creates a new kontrol instance with the given verson and config
// instance. Publickey is used for validating tokens and privateKey is used for
// signing tokens.
//
// peers can be given nil if not running on cluster.
//
// Public and private keys are RSA pem blocks that can be generated with the
// following command:
//     openssl genrsa -out testkey.pem 2048
//     openssl rsa -in testkey.pem -pubout > testkey_pub.pem
//
func New(conf *config.Config, version, publicKey, privateKey string) *Kontrol {
	k := kite.New("kontrol", version)
	k.Config = conf

	// Listen on 4000 by default
	if k.Config.Port == 0 {
		k.Config.Port = DefaultPort
	}

	hostname := k.Kite().Hostname

	kontrol := &Kontrol{
		Kite:         k,
		Name:         hostname,
		DataDir:      "kontrol-data." + hostname,
		EtcdAddr:     "localhost:4001",
		EtcdBindAddr: "0.0.0.0:4001",
		PeerAddr:     "localhost:7001",
		PeerBindAddr: "0.0.0.0:7001",
		Peers:        nil,
		publicKey:    publicKey,
		privateKey:   privateKey,
		watchers:     make(map[string]*store.Watcher),
		clients:      make(map[string]*kite.Client),
		etcd:         etcd.New(nil), // create with default config, we'll update it when running kontrol.
	}

	log = k.Log

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("registerMachine", kontrol.handleMachine).DisableAuthentication()
	k.HandleFunc("getKites", kontrol.handleGetKites)
	k.HandleFunc("getToken", kontrol.handleGetToken)
	k.HandleFunc("cancelWatcher", kontrol.handleCancelWatcher)

	k.OnFirstRequest(func(c *kite.Client) { kontrol.clients[c.ID] = c })
	k.OnDisconnect(func(c *kite.Client) { delete(kontrol.clients, c.ID) })

	return kontrol
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.Kite.Authenticators[keyType] = fn
}

func (k *Kontrol) Run() {
	k.init()
	k.Kite.Run()
}

// Close stops kontrol and closes all connections
func (k *Kontrol) Close() {
	k.etcd.Stop()
	k.Kite.Close()
}

// init does common operations of Run() and Start().
func (k *Kontrol) init() {
	rand.Seed(time.Now().UnixNano())

	// Update etcd config before running
	k.etcd.Config.Name = k.Name
	k.etcd.Config.DataDir = k.DataDir
	k.etcd.Config.Addr = k.EtcdAddr
	k.etcd.Config.BindAddr = k.EtcdBindAddr
	k.etcd.Config.Peer.Addr = k.PeerAddr
	k.etcd.Config.Peer.BindAddr = k.PeerBindAddr
	k.etcd.Config.Peers = k.Peers

	go k.etcd.Run()
	<-k.etcd.ReadyNotify()

	go k.registerSelf()
}

// ClearKites removes everything under "/kites" from etcd.
func (k *Kontrol) ClearKites() error {
	_, err := k.etcd.Store.Delete(
		KitesPrefix, // path
		true,        // recursive
		true,        // dir
	)
	if err != nil && err.(*etcdErr.Error).ErrorCode != etcdErr.EcodeKeyNotFound {
		return fmt.Errorf("Cannot clear etcd: %s", err)
	}
	return nil
}

// RegisterSelf registers this host and writes a key to ~/.kite/kite.key
func (k *Kontrol) RegisterSelf() error {
	key, err := k.registerUser(k.Kite.Config.Username)
	if err != nil {
		return err
	}
	return kitekey.Write(key)
}

func (k *Kontrol) handleMachine(r *kite.Request) (interface{}, error) {
	if k.MachineAuthenticate != nil {
		if err := k.MachineAuthenticate(r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	username := r.Args.One().MustString() // username should be send as an argument
	return k.registerUser(username)
}

func (k *Kontrol) registerUser(username string) (kiteKey string, err error) {
	// Only accept requests of type machine
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("cannot generate a token")
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        k.Kite.Kite().Username,            // Issuer
		"sub":        username,                          // Subject
		"iat":        time.Now().UTC().Unix(),           // Issued At
		"jti":        tknID.String(),                    // JWT ID
		"kontrolURL": k.Kite.Config.KontrolURL.String(), // Kontrol URL
		"kontrolKey": strings.TrimSpace(k.publicKey),    // Public key of kontrol
	}

	k.Kite.Log.Info("Registered machine on user: %s", username)

	return token.SignedString([]byte(k.privateKey))
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
		log.Error("etcd setKey error: %s", err)
		return errors.New("internal error - register")
	}

	if err := requestHeartbeat(r, setKey); err != nil {
		return err
	}

	log.Info("Kite registered: %s", r.Kite)

	r.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		k.etcd.Store.Delete(
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
		dnode.Callback(func(args *dnode.Partial) { setterFunc() }),
	}

	_, err := r.TellWithTimeout("kite.heartbeat", 4*time.Second, heartbeatArgs...)
	return err
}

// registerSelf adds Kontrol itself to etcd as a kite.
func (k *Kontrol) registerSelf() {
	value := &registerValue{
		URL: &protocol.KiteURL{*k.Kite.Config.KontrolURL},
	}
	setter, _ := k.makeSetter(k.Kite.Kite(), value)
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
		_, err := k.etcd.Store.Set(
			etcdKey,     // path
			false,       // dir
			valueString, // value
			expireAt,    // expire time
		)
		if err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		_, err = k.etcd.Store.Update(
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

var keyOrder = []string{"username", "environment", "name", "version", "region", "hostname", "id"}

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

	// http://golang.org/doc/go1.3#map, order is important and we can't rely on
	// maps because the keys are not ordered :)
	for _, key := range keyOrder {
		v := fields[key]
		if v == "" {
			empty = true
			empytField = key
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

func getAudience(q protocol.KontrolQuery) string {
	if q.Name != "" {
		return "/" + q.Username + "/" + q.Environment + "/" + q.Name
	} else if q.Environment != "" {
		return "/" + q.Username + "/" + q.Environment
	} else {
		return "/" + q.Username
	}
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	// This type is here until inversion branch is merged.
	// Reason: We can't use the same struct for marshaling and unmarshaling.
	// TODO use the struct in protocol
	type GetKitesArgs struct {
		Query         protocol.KontrolQuery  `json:"query"`
		WatchCallback dnode.Function         `json:"watchCallback"`
		Who           map[string]interface{} `json:"who"`
	}

	var args GetKitesArgs
	r.Args.One().MustUnmarshal(&args)

	if len(args.Who) != 0 {
		// Find all kites in the query and pick one.
		// TODO do not allow "who" and "watchCallback" fields to be set at the same time.
		allKites, err := k.getKites(r, args.Query, args.WatchCallback)
		if err != nil {
			return nil, err
		}
		if len(allKites.Kites) == 0 {
			return allKites, err
		}

		// We pick the first one because they come in random order.
		whoKite := allKites.Kites[0]

		// We will call the "kite.who" method of the selected kite.
		// If the kite is connected to us, we can use the existing connection.
		// Otherwise we need to open a new connection to the selected kite.
		// TODO This approach will NOT work when there are more than one kontrol instance.
		whoClient := k.clients[whoKite.Kite.ID]
		if whoClient == nil {
			// TODO Enable code below after fix.
			return nil, errors.New("target kite is not connected")
			// whoClient = k.Kite.NewClient(whoKite.URL)
			// whoClient.Authentication = &kite.Authentication{Type: "token", Key: whoKite.Token}
			// whoClient.Kite = whoKite.Kite

			// err = whoClient.Dial()
			// if err != nil {
			// 	return nil, err
			// }
			// defer whoClient.Close()
		}

		result, err := whoClient.TellWithTimeout("kite.who", 4*time.Second, args.Who)
		if err != nil {
			return nil, err
		}

		// Replace the original query with the query returned from kite.who method.
		var whoResult protocol.WhoResult
		result.MustUnmarshal(&whoResult)
		args.Query = whoResult.Query
	}

	return k.getKites(r, args.Query, args.WatchCallback)
}

func (k *Kontrol) getKites(r *kite.Request, query protocol.KontrolQuery, watchCallback dnode.Function) (*protocol.GetKitesResult, error) {
	var hasVersionConstraint bool // does query contains a constraint on version?
	var keyRest string            // query key after the version field (not including version)

	// We will make a get request to etcd store with this key.
	etcdKey, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	// audience will go into the token as "aud" claim.
	audience := getAudience(query)

	// If version field contains a constraint we need no make a new query
	// up to "name" field and filter the results after getting all versions.
	versionConstraint, err := version.NewConstraint(query.Version)
	if err == nil {
		hasVersionConstraint = true
		nameQuery := &protocol.KontrolQuery{
			Username:    query.Username,
			Environment: query.Environment,
			Name:        query.Name,
		}
		// We will make a get request to all nodes under this name
		// and filter the result later.
		etcdKey, _ = getQueryKey(nameQuery)

		// Rest of the key after version field
		keyRest = "/" + strings.TrimRight(query.Region+"/"+query.Hostname+"/"+query.ID, "/")
	}

	// Generate token once here because we are using the same token for every
	// kite we return and generating many tokens is really slow.
	token, err := generateToken(audience, r.Username, k.Kite.Kite().Username, k.privateKey)
	if err != nil {
		return nil, err
	}

	var result = new(protocol.GetKitesResult) // to be returned

	// Create e watcher on query.
	// The callback is going to be called when a Kite registered/unregistered
	// matching the query.
	// Registering watcher should be done before making etcd.Get() because
	// Get() may return an empty result.
	if watchCallback.Caller != nil {
		watcher, err := k.etcd.Store.Watch(
			KitesPrefix+etcdKey, // prefix
			true, // recursive
			true, // stream
			0,    // since index
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

		go k.watchAndSendKiteEvents(watcher, result.WatcherID, disconnect, etcdKey, watchCallback, token, hasVersionConstraint, versionConstraint, keyRest)
	}

	// Get kites from etcd
	event, err := k.etcd.Store.Get(
		KitesPrefix+etcdKey, // path
		true,  // recursive, return all child directories too
		false, // sorting flag, we don't care about sorting for now
	)

	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			result.Kites = make([]*protocol.KiteWithToken, 0) // do not send null
			return result, nil
		}

		log.Error("etcd error: %s", err)
		return nil, fmt.Errorf("internal error - getKites")
	}

	// Get all nodes recursively.
	nodes := flatten(event.Node.Nodes)

	// Convert etcd nodes to kites.
	kites := make([]*protocol.KiteWithToken, len(nodes))
	for i, n := range nodes {
		kites[i], err = kiteWithTokenFromEtcdNode(n, token)
		if err != nil {
			return nil, err
		}
	}

	// Filter kites by version constraint
	var filtered []*protocol.KiteWithToken
	if hasVersionConstraint {
		for _, k := range kites {
			if isValid(&k.Kite, versionConstraint, keyRest) {
				filtered = append(filtered, k)
			}
		}
	} else {
		filtered = kites
	}

	// Attach tokens to kites
	for _, k := range filtered {
		k.Token = token
	}

	// Shuffle the list
	shuffled := make([]*protocol.KiteWithToken, len(filtered))
	for i, v := range rand.Perm(len(filtered)) {
		shuffled[v] = filtered[i]
	}

	result.Kites = shuffled
	return result, nil
}

func isValid(k *protocol.Kite, c version.Constraints, keyRest string) bool {
	// Check the version constraint.
	v, _ := version.NewVersion(k.Version)
	if !c.Check(v) {
		return false
	}

	// Check the fields after version field.
	kiteKeyAfterVersion := "/" + strings.TrimRight(k.Region+"/"+k.Hostname+"/"+k.ID, "/")
	if !strings.HasPrefix(kiteKeyAfterVersion, keyRest) {
		return false
	}

	return true
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

// TODO watchAndSendKiteEvents takes too many arguments. Refactor it.
func (k *Kontrol) watchAndSendKiteEvents(watcher *store.Watcher, watcherID string, disconnect chan bool, etcdKey string, callback dnode.Function, token string, hasConstraint bool, constraint version.Constraints, keyRest string) {
	var index uint64 = 0
	for {
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
				var err error
				watcher, err = k.etcd.Store.Watch(
					KitesPrefix+etcdKey, // prefix
					true,  // recursive
					true,  // stream
					index, // since index
				)
				if err != nil {
					log.Error("Cannot re-watch query: %s", err.Error())
					callback.Call(kite.Response{
						Error: &kite.Error{
							Type:    "watchError",
							Message: err.Error(),
						},
					})
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

				otherKite, err := kiteWithTokenFromEtcdNode(etcdEvent.Node, token)
				if err != nil {
					continue
				}

				if hasConstraint && !isValid(&otherKite.Kite, constraint, keyRest) {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Register
				e.Kite = otherKite.Kite
				e.URL = otherKite.URL
				e.Token = otherKite.Token

				callback.Call(kite.Response{Result: e})

			// Delete happens when we detect that otherKite is disconnected.
			// Expire happens when we don't get heartbeat from otherKite.
			case store.Delete, store.Expire:
				otherKite, err := kiteFromEtcdKey(etcdEvent.Node.Key)
				if err != nil {
					continue
				}

				if hasConstraint && !isValid(otherKite, constraint, keyRest) {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Deregister
				e.Kite = *otherKite

				callback.Call(kite.Response{Result: e})
			}
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

func kitesFromNodes(nodes store.NodeExterns) ([]*protocol.KiteWithToken, error) {
	kites := make([]*protocol.KiteWithToken, len(nodes))

	for i, node := range nodes {
		var rv registerValue
		json.Unmarshal([]byte(*node.Value), &rv)

		kite, _ := kiteFromEtcdKey(node.Key)

		kites[i] = &protocol.KiteWithToken{
			Kite: *kite,
			URL:  rv.URL.String(),
		}
	}

	return kites, nil
}

func kiteWithTokenFromEtcdNode(node *store.NodeExtern, token string) (*protocol.KiteWithToken, error) {
	kite, err := kiteFromEtcdKey(node.Key)
	if err != nil {
		return nil, err
	}

	var rv registerValue
	err = json.Unmarshal([]byte(*node.Value), &rv)
	if err != nil {
		return nil, err
	}

	return &protocol.KiteWithToken{
		Kite:  *kite,
		URL:   rv.URL.String(),
		Token: token,
	}, nil
}

// generateToken returns a JWT token string. Please see the URL for details:
// http://tools.ietf.org/html/draft-ietf-oauth-json-web-token-13#section-4.1
func generateToken(queryKey, username, issuer, privateKey string) (string, error) {
	tokenCacheMu.Lock()
	defer tokenCacheMu.Unlock()

	uniqKey := queryKey + username + issuer // neglect privateKey, its always the same
	signed, ok := tokenCache[uniqKey]
	if ok {
		return signed, nil
	}

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

	signed, err = tkn.SignedString([]byte(privateKey))
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	// cache our token
	tokenCache[uniqKey] = signed

	// cache invalidation, because we cache the token in tokenCache we need to
	// invalidate it expiration time. This was handled usually within JWT, but
	// now we have to do it manually for our own cache.
	time.AfterFunc(TokenTTL, func() {
		tokenCacheMu.Lock()
		defer tokenCacheMu.Unlock()

		delete(tokenCache, uniqKey)
	})

	return signed, nil
}

// kiteFromEtcdKey returns a *protocol.Kite from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKey(key string) (*protocol.Kite, error) {
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

	_, err = k.etcd.Store.Get(
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

	return generateToken(kiteKey, r.Username, k.Kite.Kite().Username, k.privateKey)
}
