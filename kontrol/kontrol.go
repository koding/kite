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

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	uuid "github.com/satori/go.uuid"
)

const (
	KontrolVersion = "0.0.4"
	KitesPrefix    = "/kites"
)

var (
	TokenTTL    = 48 * time.Hour
	TokenLeeway = 1 * time.Minute
	DefaultPort = 4000

	tokenCache   = make(map[string]string)
	tokenCacheMu sync.Mutex

	// HeartbeatInterval is the interval in which kites are sending heartbeats
	HeartbeatInterval = time.Second * 10

	// HeartbeatDelay is the compensation interval which is added to the
	// heartbeat to avoid network delays
	HeartbeatDelay = time.Second * 20

	// UpdateInterval is the interval in which the key gets updated
	// periodically. Keeping it low increase the write load to the storage, so
	// be cautious when changing it.
	UpdateInterval = time.Second * 60

	// KeyTLL is the timeout in which a key expires. Each storage
	// implementation needs to set keys according to this Key. If a storage
	// doesn't support TTL mechanism (such as PostgreSQL), it should use a
	// background cleaner which cleans up keys that are KeyTTL old.
	KeyTTL = time.Second * 90
)

type Kontrol struct {
	Kite *kite.Kite

	// MachineAuthenticate is used to authenticate the request in the
	// "handleMachine" method.  The reason for a separate auth function is, the
	// request must not be authenticated because clients do not have a kite.key
	// before they register to this machine. Also the requester can send a
	// authType argument which can be used to distinguish between several
	// authentication methods
	MachineAuthenticate func(authType string, r *kite.Request) error

	// MachineKeyPicker is used to choose the key pair to generate a valid
	// kite.key file for the "handleMachine" method. This overrides the default
	// last keypair added with kontrol.AddKeyPair method.
	MachineKeyPicker func(r *kite.Request) (*KeyPair, error)

	clientLocks *IdLock

	heartbeats   map[string]*time.Timer
	heartbeatsMu sync.Mutex // protects each clients heartbeat timer

	// closed notifies goroutines started by kontrol that it got closed
	closed chan struct{}

	// keyPair defines the storage of keypairs
	keyPair KeyPairStorage

	// ids, lastPublic and lastPrivate are used to store the last added keys
	// for convinience
	lastIDs     []string
	lastPublic  []string
	lastPrivate []string

	// storage defines the storage of the kites.
	storage Storage

	// RegisterURL defines the URL that is used to self register when adding
	// itself to the storage backend
	RegisterURL string

	log kite.Logger
}

// New creates a new kontrol instance with the given version and config
// instance, and the default kontrol handlers. Publickey is used for
// validating tokens and privateKey is used for signing tokens.
//
// Public and private keys are RSA pem blocks that can be generated with the
// following command:
//     openssl genrsa -out testkey.pem 2048
//     openssl rsa -in testkey.pem -pubout > testkey_pub.pem
//
// If you need to provide custom handlers in place of the default ones,
// use the following command instead:
//     NewWithoutHandlers(conf, version)
//
func New(conf *config.Config, version string) *Kontrol {
	kontrol := NewWithoutHandlers(conf, version)

	kontrol.Kite.HandleFunc("register", kontrol.HandleRegister)
	kontrol.Kite.HandleFunc("registerMachine", kontrol.HandleMachine).DisableAuthentication()
	kontrol.Kite.HandleFunc("getKites", kontrol.HandleGetKites)
	kontrol.Kite.HandleFunc("getToken", kontrol.HandleGetToken)
	kontrol.Kite.HandleFunc("getKey", kontrol.HandleGetKey)

	kontrol.Kite.HandleHTTPFunc("/register", kontrol.HandleRegisterHTTP)
	kontrol.Kite.HandleHTTPFunc("/heartbeat", kontrol.HandleHeartbeat)

	return kontrol
}

// NewWithoutHandlers creates a new kontrol instance with the given version and config
// instance, but *without* the default handlers. If this is function is
// used, make sure to implement the expected kontrol functionality.
//
// Example:
//
//     kontrol := NewWithoutHandlers(conf, version)
//     kontrol.Kite.HandleFunc("register", kontrol.HandleRegister)
//     kontrol.Kite.HandleFunc("registerMachine", kontrol.HandleMachine).DisableAuthentication()
//     kontrol.Kite.HandleFunc("getKites", kontrol.HandleGetKites)
//     kontrol.Kite.HandleFunc("getToken", kontrol.HandleGetToken)
//     kontrol.Kite.HandleFunc("getKey", kontrol.HandleGetKey)
//     kontrol.Kite.HandleHTTPFunc("/heartbeat", kontrol.HandleHeartbeat)
//     kontrol.Kite.HandleHTTPFunc("/register", kontrol.HandleRegisterHTTP)
//
func NewWithoutHandlers(conf *config.Config, version string) *Kontrol {
	k := kite.New("kontrol", version)
	k.Config = conf

	// Listen on 4000 by default
	if k.Config.Port == 0 {
		k.Config.Port = DefaultPort
	}

	k.SetLogLevel(kite.DEBUG)

	return &Kontrol{
		Kite:        k,
		log:         k.Log,
		clientLocks: NewIdlock(),
		heartbeats:  make(map[string]*time.Timer, 0),
		closed:      make(chan struct{}),
		lastIDs:     make([]string, 0),
		lastPublic:  make([]string, 0),
		lastPrivate: make([]string, 0),
	}
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.Kite.Authenticators[keyType] = fn
}

// DeleteKeyPair deletes the key with the given id or public key. (One of them
// can be empty)
func (k *Kontrol) DeleteKeyPair(id, public string) error {
	if k.keyPair == nil {
		return errors.New("Key pair storage is not initialized")
	}

	pair, err := k.keyPair.GetKeyFromID(id)
	if err != nil {
		return err
	}

	k.keyPair.DeleteKey(&KeyPair{
		ID:     id,
		Public: public,
	})

	// if public is empty
	if public == "" {
		public = pair.Public
	}

	deleteIndex := -1
	for i, p := range k.lastPublic {
		if p == public {
			deleteIndex = i
			break
		}
	}

	if deleteIndex == -1 {
		return errors.New("deleteKeyPair: public key not found")
	}

	// delete the given public key
	k.lastIDs = append(k.lastIDs[:deleteIndex], k.lastIDs[deleteIndex+1:]...)
	k.lastPublic = append(k.lastPublic[:deleteIndex], k.lastPublic[deleteIndex+1:]...)
	k.lastPrivate = append(k.lastPrivate[:deleteIndex], k.lastPrivate[deleteIndex+1:]...)

	return nil
}

// AddKeyPair add the given key pair so it can be used to validate and
// sign/generate tokens. If id is empty, a unique ID will be generated. The
// last added key pair is also used to generate tokens for machine
// registrations via "handleMachine" method. This can be overiden with the
// kontorl.MachineKeyPicker function.
func (k *Kontrol) AddKeyPair(id, public, private string) error {
	if k.keyPair == nil {
		k.log.Warning("Key pair storage is not set. Using in memory cache")
		k.keyPair = NewMemKeyPairStorage()
	}

	if id == "" {
		i := uuid.NewV4()
		id = i.String()
	}

	public = strings.TrimSpace(public)
	private = strings.TrimSpace(private)

	keyPair := &KeyPair{
		ID:      id,
		Public:  public,
		Private: private,
	}

	// set last set key pair
	k.lastIDs = append(k.lastIDs, id)
	k.lastPublic = append(k.lastPublic, public)
	k.lastPrivate = append(k.lastPrivate, private)

	if err := keyPair.Validate(); err != nil {
		return err
	}

	return k.keyPair.AddKey(keyPair)
}

func (k *Kontrol) Run() {
	rand.Seed(time.Now().UnixNano())

	if k.storage == nil {
		panic("kontrol storage is not set")
	}

	if k.keyPair == nil {
		k.log.Warning("Key pair storage is not set. Using in memory cache")
		k.keyPair = NewMemKeyPairStorage()
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

// SetKeyPairStorage sets the backend storage that kontrol is going to use to
// store keypairs
func (k *Kontrol) SetKeyPairStorage(storage KeyPairStorage) {
	k.keyPair = storage
}

// Close stops kontrol and closes all connections
func (k *Kontrol) Close() {
	close(k.closed)
	k.Kite.Close()
}

// InitializeSelf registers his host by writing a key to ~/.kite/kite.key
func (k *Kontrol) InitializeSelf() error {
	if len(k.lastPublic) == 0 && len(k.lastPrivate) == 0 {
		return errors.New("Please initialize AddKeyPair() method")
	}

	key, err := k.registerUser(k.Kite.Config.Username, k.lastPublic[0], k.lastPrivate[0])
	if err != nil {
		return err
	}
	return kitekey.Write(key)
}

func (k *Kontrol) registerUser(username, publicKey, privateKey string) (kiteKey string, err error) {
	// Only accept requests of type machine
	tknID := uuid.NewV4()

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        k.Kite.Kite().Username,       // Issuer
		"sub":        username,                     // Subject
		"iat":        time.Now().UTC().Unix(),      // Issued At
		"jti":        tknID.String(),               // JWT ID
		"kontrolURL": k.Kite.Config.KontrolURL,     // Kontrol URL
		"kontrolKey": strings.TrimSpace(publicKey), // Public key of kontrol
	}

	k.Kite.Log.Info("Registered machine on user: %s", username)

	return token.SignedString([]byte(privateKey))
}

// registerSelf adds Kontrol itself to the storage as a kite.
func (k *Kontrol) registerSelf() {
	value := &kontrolprotocol.RegisterValue{
		URL: k.Kite.Config.KontrolURL,
	}

	// change if the user wants something different
	if k.RegisterURL != "" {
		value.URL = k.RegisterURL
	}

	keyPair, err := k.selfKeyPair()
	if err != nil {
		if err != errNoSelfKeyPair {
			k.log.Error("%s", err)
		}

		// If Kontrol does not hold any key pairs that was used
		// to generate its kitekey or no kitekey is defined,
		// use a dummy entry in order to register the kontrol.
		keyPair = &KeyPair{
			ID:      uuid.NewV4().String(),
			Public:  "kontrol-self",
			Private: "kontrol-self",
		}

		if err := k.keyPair.AddKey(keyPair); err != nil {
			k.log.Error("%s", err)
		}
	}

	if pair, err := k.keyPair.GetKeyFromPublic(keyPair.Public); err == nil {
		keyPair = pair
	}

	value.KeyID = keyPair.ID

	// Register first by adding the value to the storage. We don't return any
	// error because we need to know why kontrol doesn't register itself
	if err := k.storage.Add(k.Kite.Kite(), value); err != nil {
		k.log.Error("%s", err)
	}

	for {
		select {
		case <-k.closed:
			return
		default:
			if err := k.storage.Update(k.Kite.Kite(), value); err != nil {
				k.log.Error("%s", err)
				time.Sleep(time.Second)
				continue
			}

			time.Sleep(HeartbeatDelay + HeartbeatInterval)
		}
	}
}

func (k *Kontrol) selfKeyPair() (pair *KeyPair, err error) {
	if k.Kite.Config.KiteKey == "" || len(k.lastPublic) == 0 {
		return nil, errNoSelfKeyPair
	}

	keyIndex := -1

	me := new(multiError)

	for i := range k.lastPublic {
		ri := len(k.lastPublic) - i - 1

		keyFn := func(token *jwt.Token) (interface{}, error) {
			return []byte(k.lastPublic[ri]), nil
		}

		if _, err := jwt.Parse(k.Kite.Config.KiteKey, keyFn); err != nil {
			me.err = append(me.err, err)
			continue
		}

		keyIndex = ri
		break
	}

	if keyIndex == -1 {
		return nil, fmt.Errorf("no matching self key pair found: %s", me)
	}

	return &KeyPair{
		ID:      k.lastIDs[keyIndex],
		Public:  k.lastPublic[keyIndex],
		Private: k.lastPrivate[keyIndex],
	}, nil
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

	tknID := uuid.NewV4()

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

	var err error
	signed, err = tkn.SignedString([]byte(privateKey))
	if err != nil {
		return "", errors.New("Server error: Cannot generate a token")
	}

	// cache our token
	tokenCache[uniqKey] = signed

	// cache invalidation, because we cache the token in tokenCache we need to
	// invalidate it expiration time. This was handled usually within JWT, but
	// now we have to do it manually for our own cache.
	time.AfterFunc(TokenTTL-TokenLeeway, func() {
		tokenCacheMu.Lock()
		defer tokenCacheMu.Unlock()

		delete(tokenCache, uniqKey)
	})

	return signed, nil
}
