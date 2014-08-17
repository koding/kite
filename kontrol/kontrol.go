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
	"github.com/dgrijalva/jwt-go"
	"github.com/hashicorp/go-version"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kitekey"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
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

	// MachineAuthenticate is used to authenticate the request in the
	// "handleMachine" method.  The reason for a separate auth function is, the
	// request must not be authenticated because clients do not have a kite.key
	// before they register to this machine.
	MachineAuthenticate func(r *kite.Request) error

	// RSA keys
	publicKey  string // for validating tokens
	privateKey string // for signing tokens

	// To cancel running watchers, we must store the references
	watchers      map[string]*Watcher
	watchersMutex sync.Mutex

	// Holds refence to all connected clients (key is ID of kite)
	clients map[string]*kite.Client

	// storage defines the storage of the kites.
	storage Storage

	// RegisterURL defines the URL that is used to self register when adding
	// itself to the storage backend
	RegisterURL string

	// a list of etcd machintes to connect
	Machines []string
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

	kontrol := &Kontrol{
		Kite:       k,
		publicKey:  publicKey,
		privateKey: privateKey,
		watchers:   make(map[string]*Watcher),
		clients:    make(map[string]*kite.Client),
	}

	log = k.Log

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("registerMachine", kontrol.handleMachine).DisableAuthentication()
	k.HandleFunc("getKites", kontrol.handleGetKites)
	k.HandleFunc("getToken", kontrol.handleGetToken)
	k.HandleFunc("cancelWatcher", kontrol.handleCancelWatcher)

	var mu sync.Mutex
	k.OnFirstRequest(func(c *kite.Client) {
		mu.Lock()
		kontrol.clients[c.ID] = c
		mu.Unlock()
	})

	k.OnDisconnect(func(c *kite.Client) {
		mu.Lock()
		delete(kontrol.clients, c.ID)
		mu.Unlock()
	})

	return kontrol
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.Kite.Authenticators[keyType] = fn
}

func (k *Kontrol) Run() {
	rand.Seed(time.Now().UnixNano())

	// assume we are going to work locally instead of panicing
	if k.Machines == nil || len(k.Machines) == 0 {
		k.Machines = []string{"127.0.0.1:4001"}
	}

	k.Kite.Log.Info("Connecting to Etcd with machines: %v", k.Machines)
	etcdClient, err := NewEtcd(k.Machines)
	if err != nil {
		panic("could not connect to etcd: " + err.Error())
	}
	etcdClient.log = k.Kite.Log

	k.storage = etcdClient

	// now go and register ourself
	go k.registerSelf()

	k.Kite.Run()
}

// Close stops kontrol and closes all connections
func (k *Kontrol) Close() {
	k.Kite.Close()
}

// ClearKites removes everything under "/kites" from etcd.
func (k *Kontrol) ClearKites() error {
	err := k.storage.Delete(KitesPrefix)
	if err != nil && err.(*etcdErr.Error).ErrorCode != etcdErr.EcodeKeyNotFound {
		return fmt.Errorf("Cannot clear etcd: %s", err)
	}

	return nil
}

// InitializeSelf registers his host by writing a key to ~/.kite/kite.key
func (k *Kontrol) InitializeSelf() error {
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
		"iss":        k.Kite.Kite().Username,         // Issuer
		"sub":        username,                       // Subject
		"iat":        time.Now().UTC().Unix(),        // Issued At
		"jti":        tknID.String(),                 // JWT ID
		"kontrolURL": k.Kite.Config.KontrolURL,       // Kontrol URL
		"kontrolKey": strings.TrimSpace(k.publicKey), // Public key of kontrol
	}

	k.Kite.Log.Info("Registered machine on user: %s", username)

	return token.SignedString([]byte(k.privateKey))
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %s", r.Client.Kite)

	if r.Args.One().MustMap()["url"].MustString() == "" {
		return nil, errors.New("invalid url")
	}

	var args struct {
		URL string `json:"url"`
	}
	r.Args.One().MustUnmarshal(&args)
	if args.URL == "" {
		return nil, errors.New("empty url")
	}

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Auth.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Auth.Type)
	}

	err := k.register(r.Client, args.URL)
	if err != nil {
		return nil, err
	}

	// send response back to the kite, also identify him with the new name
	return &protocol.RegisterResult{URL: args.URL}, nil
}

func (k *Kontrol) register(r *kite.Client, kiteURL string) error {
	if err := validateKiteKey(&r.Kite); err != nil {
		return err
	}

	queryKey, err := GetQueryKey(r.Query())
	if err != nil {
		return err
	}

	// Register only if this kite is not already registered.
	// err == nil means there is no error, so there is a key. there shouldn't be.
	_, err = k.storage.Get(KitesPrefix + queryKey)
	if err == nil {
		return errors.New(fmt.Sprintf("There is a kite already registered with the same settings: %s", queryKey))
	}

	value := &kontrolprotocol.RegisterValue{
		URL: kiteURL,
	}

	// setKey sets the value of the Kite in etcd.
	setKey, etcdKey, etcdIDKey := k.makeSetter(&r.Kite, value)

	// Register to etcd.
	if err = setKey(); err != nil {
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
		k.storage.Delete(etcdKey)
		// And the Id
		k.storage.Delete(etcdIDKey)
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
	value := &kontrolprotocol.RegisterValue{
		URL: k.Kite.Config.KontrolURL,
	}

	// change if the user wants something different
	if k.RegisterURL != "" {
		value.URL = k.RegisterURL
	}

	setter, _, _ := k.makeSetter(k.Kite.Kite(), value)
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
func (k *Kontrol) makeSetter(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) (setter func() error, etcdKey, etcdIDKey string) {
	etcdKey = KitesPrefix + kite.String()
	etcdIDKey = KitesPrefix + "/" + kite.ID

	valueBytes, _ := json.Marshal(value)
	valueString := string(valueBytes)

	setter = func() error {
		// Set the kite key.
		// Example "/koding/production/os/0.0.1/sj/kontainer1.sj.koding.com/1234asdf..."
		if err := k.storage.Set(etcdKey, valueString); err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		// Also store the the kite.Key Id for easy lookup
		if err := k.storage.Set(etcdIDKey, kite.String()); err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		if err := k.storage.Update(KitesPrefix+"/"+kite.Username, ""); err != nil {
			log.Error("etcd error (3): %s", err)
			return err
		}

		return nil
	}

	return
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

	// We will make a get request to etcd store with this key. Check first if
	etcdKey, err := k.getEtcdKey(&query)
	if err != nil {
		return nil, err
	}

	// audience will go into the token as "aud" claim.
	audience := getAudience(query)

	// If version field contains a constraint we need no make a new query up to
	// "name" field and filter the results after getting all versions.
	// NewVersion returns an error if it's a constraint, like: ">= 1.0, < 1.4"
	// Because NewConstraint doesn't return an error for version's like "0.0.1"
	// we check it with the NewVersion function.
	var versionConstraint version.Constraints
	_, err = version.NewVersion(query.Version)
	if err != nil && query.Version != "" {
		// now parse our constraint
		versionConstraint, err = version.NewConstraint(query.Version)
		if err != nil {
			// version is a malformed, just return the error
			return nil, err
		}

		hasVersionConstraint = true
		nameQuery := &protocol.KontrolQuery{
			Username:    query.Username,
			Environment: query.Environment,
			Name:        query.Name,
		}
		// We will make a get request to all nodes under this name
		// and filter the result later.
		etcdKey, _ = GetQueryKey(nameQuery)

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
		k.Kite.Log.Info("Watcher enabled for query: %s", query)
		watcher, err := k.storage.Watch(KitesPrefix+etcdKey, 0)
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
			k.Kite.Log.Info("Watcher client disconnected, closing the watcher")
			// Remove watcher from the map
			k.watchersMutex.Lock()
			defer k.watchersMutex.Unlock()
			delete(k.watchers, watcherID.String())

			// Notify disconnection and stop watching.
			close(disconnect)
			watcher.Stop()
		})
		k.watchersMutex.Unlock()

		go k.watchAndSendKiteEvents(watcher, result.WatcherID, disconnect, etcdKey, watchCallback, token, hasVersionConstraint, versionConstraint, keyRest)
	}

	// Get kites from etcd
	node, err := k.storage.Get(KitesPrefix + etcdKey)
	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			result.Kites = make([]*protocol.KiteWithToken, 0) // do not send null
			return result, nil
		}

		log.Error("etcd error: %s", err)
		return nil, fmt.Errorf("internal error - getKites")
	}

	// means a query with all fields were made or a query with an ID was made,
	// in which case also returns a full path. This path has a value that
	// contains the final kite URL. Therefore this is a single kite result,
	// create it and pass it back.
	if node.HasValue() {
		kiteWithToken, err := node.Kite()
		if err != nil {
			return nil, err
		}

		// attach our generated token
		kiteWithToken.Token = token

		result.Kites = []*protocol.KiteWithToken{kiteWithToken}
		return result, nil
	}

	// we have a tree of nodes. Get all the kites under the current tree
	kites, err := node.Kites()
	if err != nil {
		return nil, err
	}

	// Filter kites by version constraint
	if hasVersionConstraint {
		kites.Filter(versionConstraint, keyRest)
	}

	// Shuffle the list
	kites.Shuffle()

	// Attach tokens to kites
	kites.Attach(token)

	result.Kites = kites
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

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var query protocol.KontrolQuery
	err := r.Args.One().Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query")
	}

	kiteKey, err := k.getEtcdKey(&query)
	if err != nil {
		return nil, err
	}

	_, err = k.storage.Get(KitesPrefix + kiteKey)
	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			return nil, errors.New("Kite not found")
		}
		return nil, err
	}

	return generateToken(kiteKey, r.Username, k.Kite.Kite().Username, k.privateKey)
}
