// Package kontrol provides an implementation for the name service kite.
// It can be queried to get the list of running kites.
package kontrol

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/nu7hatch/gouuid"
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

	// RSA keys
	publicKey  string // for validating tokens
	privateKey string // for signing tokens

	clientLocks *IdLock

	heartbeats   map[string]*time.Timer
	heartbeatsMu sync.Mutex // protects each clients heartbeat timer

	// storage defines the storage of the kites.
	storage Storage

	// RegisterURL defines the URL that is used to self register when adding
	// itself to the storage backend
	RegisterURL string

	log kite.Logger
}

// New creates a new kontrol instance with the given version and config
// instance. Publickey is used for validating tokens and privateKey is used for
// signing tokens.
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
		Kite:        k,
		publicKey:   publicKey,
		privateKey:  privateKey,
		log:         k.Log,
		clientLocks: NewIdlock(),
		heartbeats:  make(map[string]*time.Timer, 0),
	}

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("registerMachine", kontrol.handleMachine).DisableAuthentication()
	k.HandleFunc("getKites", kontrol.handleGetKites)
	k.HandleFunc("getToken", kontrol.handleGetToken)

	k.HandleHTTPFunc("/register", kontrol.handleRegisterHTTP)
	k.HandleHTTPFunc("/heartbeat", kontrol.handleHeartbeat)

	return kontrol
}

func (k *Kontrol) AddAuthenticator(keyType string, fn func(*kite.Request) error) {
	k.Kite.Authenticators[keyType] = fn
}

func (k *Kontrol) Run() {
	rand.Seed(time.Now().UnixNano())

	if k.storage == nil {
		panic("kontrol storage is not set")
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

// registerSelf adds Kontrol itself to the storage as a kite.
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
		k.log.Error(err.Error())
	}

	for {
		if err := k.storage.Update(k.Kite.Kite(), value); err != nil {
			k.log.Error(err.Error())
			time.Sleep(time.Second)
			continue
		}

		time.Sleep(HeartbeatDelay + HeartbeatInterval)
	}
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
	time.AfterFunc(TokenTTL-TokenLeeway, func() {
		tokenCacheMu.Lock()
		defer tokenCacheMu.Unlock()

		delete(tokenCache, uniqKey)
	})

	return signed, nil
}
