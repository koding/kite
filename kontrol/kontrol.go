// Package kontrol provides an implementation for the name service kite.
// It can be queried to get the list of running kites.
package kontrol

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

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

	// Holds refence to all connected clients (key is ID of kite)
	clients map[string]*kite.Client

	// storage defines the storage of the kites.
	storage Storage

	// RegisterURL defines the URL that is used to self register when adding
	// itself to the storage backend
	RegisterURL string
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
		clients:    make(map[string]*kite.Client),
	}

	log = k.Log

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("registerMachine", kontrol.handleMachine).DisableAuthentication()
	k.HandleFunc("getKites", kontrol.handleGetKites)
	k.HandleFunc("getToken", kontrol.handleGetToken)

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

	if k.storage == nil {
		panic("storage is not set")
	}

	// now go and register ourself
	go k.registerSelf()

	k.Kite.Run()
}

// SetStorage sets the backend storage that kontrol is going to use to store
// kites
func (k *Kontrol) SetStorage(storage Storage) {
	k.storage = storage
}

// Close stops kontrol and closes all connections
func (k *Kontrol) Close() {
	k.Kite.Close()
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

	value := &kontrolprotocol.RegisterValue{
		URL: kiteURL,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Add(&r.Kite, value); err != nil {
		log.Error("etcd setKey error: %s", err)
		return errors.New("internal error - register")
	}

	// updater updates the value of the Kite in etcd. We are going to update
	// the value periodically so if we don't get any update we are going to
	// assume that the klient is disconnected.
	updater := k.makeUpdater(&r.Kite, value)

	if err := requestHeartbeat(r, updater); err != nil {
		return err
	}

	log.Info("Kite registered: %s", r.Kite)

	r.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		// And the Id
		k.storage.Delete(&r.Kite)
	})

	return nil
}

// requestHeartbeat is calling the remote kite's kite.heartbeat method with the
// given updaterFunc callback. The remote kite is calling this updaterFunc
// every 4 seconds
func requestHeartbeat(r *kite.Client, updaterFunc func() error) error {
	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		dnode.Callback(func(args *dnode.Partial) { updaterFunc() }),
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

	// Register first by adding the value to the storage. We don't return any
	// error because we need to know why kontrol doesn't register itself
	if err := k.storage.Add(k.Kite.Kite(), value); err != nil {
		log.Error(err.Error())
	}

	updater := k.makeUpdater(k.Kite.Kite(), value)
	for {
		if err := updater(); err != nil {
			log.Error(err.Error())
			time.Sleep(time.Second)
			continue
		}

		time.Sleep(HeartbeatInterval)
	}
}

//  makeUpdater returns a func for updating the value for the given kite key with value.
func (k *Kontrol) makeUpdater(kiteProt *protocol.Kite, value *kontrolprotocol.RegisterValue) func() error {

	return func() error {
		if err := k.storage.Update(kiteProt, value); err != nil {
			log.Error("etcd error: %s", err)
			return err
		}

		return nil
	}
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	// This type is here until inversion branch is merged.
	// Reason: We can't use the same struct for marshaling and unmarshaling.
	// TODO use the struct in protocol
	type GetKitesArgs struct {
		Query         *protocol.KontrolQuery `json:"query"`
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

func (k *Kontrol) getKites(r *kite.Request, query *protocol.KontrolQuery, watchCallback dnode.Function) (*protocol.GetKitesResult, error) {
	// audience will go into the token as "aud" claim.
	audience := getAudience(query)

	// Generate token once here because we are using the same token for every
	// kite we return and generating many tokens is really slow.
	token, err := generateToken(audience, r.Username,
		k.Kite.Kite().Username, k.privateKey)
	if err != nil {
		return nil, err
	}

	// disable this until it can be supported better with the storage interface
	if watchCallback.Caller != nil {
		return nil, errors.New("watch functionality is disabled")
	}

	// Get kites from the storage
	kites, err := k.storage.Get(query)
	if err != nil {
		return nil, err
	}

	// Attach tokens to kites
	kites.Attach(token)

	return &protocol.GetKitesResult{
		Kites: kites,
	}, nil
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
func generateToken(aud, username, issuer, privateKey string) (string, error) {
	tokenCacheMu.Lock()
	defer tokenCacheMu.Unlock()

	uniqKey := aud + username + issuer // neglect privateKey, its always the same
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
	tkn.Claims["aud"] = aud                                          // Audience
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
	var query *protocol.KontrolQuery
	err := r.Args.One().Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query")
	}

	// check if it's exist
	kites, err := k.storage.Get(query)
	if err != nil {
		return nil, err
	}

	if len(kites) > 1 {
		return nil, errors.New("query matches more than one kite")
	}

	audience := getAudience(query)

	return generateToken(audience, r.Username, k.Kite.Kite().Username, k.privateKey)
}
