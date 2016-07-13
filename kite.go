// Package kite is a library for creating micro-services.  Two main types
// implemented by this package are Kite for creating a micro-service server
// called "Kite" and Client for communicating with another kites.
package kite

import (
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/koding/cache"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/sockjsclient"
	uuid "github.com/satori/go.uuid"
	"gopkg.in/igm/sockjs-go.v2/sockjs"
)

var hostname string

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("kite: cannot get hostname: %s", err.Error()))
	}

	jwt.TimeFunc = func() time.Time {
		return time.Now().UTC()
	}
}

// Kite defines a single process that enables distributed service messaging
// amongst the peers it is connected. A Kite process acts as a Client and as a
// Server. That means it can receive request, process them, but it also can
// make request to other kites.
//
// Do not use this struct directly. Use kite.New function, add your handlers
// with HandleFunc mehtod, then call Run method to start the inbuilt server (or
// pass it to any http.Handler compatible server)
type Kite struct {
	Config *config.Config

	// Log logs with the given Logger interface
	Log Logger

	// SetLogLevel changes the level of the logger. Default is INFO.
	SetLogLevel func(Level)

	// Contains different functions for authenticating user from request.
	// Keys are the authentication types (options.auth.type).
	Authenticators map[string]func(*Request) error

	// ClientFunc is used as the default value for kite.Client.ClientFunc.
	// If nil, a default ClientFunc will be used.
	//
	// See also: kite.Client.ClientFunc docstring.
	ClientFunc func(*sockjsclient.DialOptions) *http.Client

	// Handlers added with Kite.HandleFunc().
	handlers     map[string]*Method // method map for exported methods
	preHandlers  []Handler          // a list of handlers that are executed before any handler
	postHandlers []Handler          // a list of handlers that are executed after any handler

	// MethodHandling defines how the kite is returning the response for
	// multiple handlers
	MethodHandling MethodHandling

	// HTTP muxer
	muxer *mux.Router

	// kontrolclient is used to register to kontrol and query third party kites
	// from kontrol
	kontrol *kontrolClient

	// kontrolKey stores parsed Config.KontrolKey
	kontrolKey *rsa.PublicKey

	// configMu protects access to Config.{Kite,Kontrol}Key fields.
	configMu sync.RWMutex

	// verifyCache is used as a cache for verify method.
	//
	// The field is set by verifyInit method.
	verifyCache *cache.MemoryTTL

	// verifyFunc is a verify method used to verify auth keys.
	//
	// For more details see (config.Config).VerifyFunc.
	//
	// The field is set by verifyInit method.
	verifyFunc func(pub string) error

	// verifyAudienceFunc is used to verify the audience of an
	// an incoming JWT token.
	//
	// For more details see (config.Config).VerifyAudienceFunc.
	//
	// The field is set by verifyInit method.
	verifyAudienceFunc func(*protocol.Kite, string) error

	// verifyOnce ensures all verify* fields are set up only once.
	verifyOnce sync.Once

	// mu protects assigment to verifyCache
	mu sync.Mutex

	// Handlers to call when a new connection is received.
	onConnectHandlers []func(*Client)

	// Handlers to call before the first request of connected kite.
	onFirstRequestHandlers []func(*Client)

	// Handlers to call when a client has disconnected.
	onDisconnectHandlers []func(*Client)

	// onRegisterHandlers field holds callbacks invoked when Kite
	// registers successfully to Kontrol
	onRegisterHandlers []func(*protocol.RegisterResult)

	// handlersMu protects access to on*Handlers fields.
	handlersMu sync.RWMutex

	// heartbeatC is used to control kite's heartbeats; sending
	// a non-nil value on the channel makes heartbeat goroutine issue
	// new heartbeats; sending nil value stops heartbeats
	heartbeatC chan *heartbeatReq

	// server fields, are initialized and used when
	// TODO: move them to their own struct, just like KontrolClient
	listener  *gracefulListener
	TLSConfig *tls.Config
	readyC    chan bool // To signal when kite is ready to accept connections
	closeC    chan bool // To signal when kite is closed with Close()

	name    string
	version string
	Id      string // Unique kite instance id
}

// New creates, initialize and then returns a new Kite instance. Version must
// be in 3-digit semantic form. Name is important that it's also used to be
// searched by others.
func New(name, version string) *Kite {
	if name == "" {
		panic("kite: name cannot be empty")
	}

	if digits := strings.Split(version, "."); len(digits) != 3 {
		panic("kite: version must be 3-digits semantic version")
	}

	kiteID := uuid.NewV4()

	l, setlevel := newLogger(name)

	kClient := &kontrolClient{
		readyConnected:  make(chan struct{}),
		readyRegistered: make(chan struct{}),
		registerChan:    make(chan *url.URL, 1),
	}

	k := &Kite{
		Config:         config.New(),
		Log:            l,
		SetLogLevel:    setlevel,
		Authenticators: make(map[string]func(*Request) error),
		handlers:       make(map[string]*Method),
		kontrol:        kClient,
		name:           name,
		version:        version,
		Id:             kiteID.String(),
		readyC:         make(chan bool),
		closeC:         make(chan bool),
		heartbeatC:     make(chan *heartbeatReq, 1),
		muxer:          mux.NewRouter(),
	}

	// We change the heartbeat interval from 25 seconds to 10 seconds. This is
	// better for environments such as AWS ELB.
	sockjsOpts := sockjs.DefaultOptions
	sockjsOpts.HeartbeatDelay = 10 * time.Second

	// All sockjs communication is done through this endpoint..
	k.muxer.PathPrefix("/kite").Handler(sockjs.NewHandler("/kite", sockjsOpts, k.sockjsHandler))

	// Add useful debug logs
	k.OnConnect(func(c *Client) { k.Log.Debug("New session: %s", c.session.ID()) })
	k.OnFirstRequest(func(c *Client) { k.Log.Debug("Session %q is identified as %q", c.session.ID(), c.Kite) })
	k.OnDisconnect(func(c *Client) { k.Log.Debug("Kite has disconnected: %q", c.Kite) })
	k.OnRegister(k.updateAuth)

	// Every kite should be able to authenticate the user from token.
	// Tokens are granted by Kontrol Kite.
	k.Authenticators["token"] = k.AuthenticateFromToken

	// A kite accepts requests with the same username.
	k.Authenticators["kiteKey"] = k.AuthenticateFromKiteKey

	// Register default methods and handlers.
	k.addDefaultHandlers()

	go k.processHeartbeats()

	return k
}

// Kite returns the definition of the kite.
func (k *Kite) Kite() *protocol.Kite {
	return &protocol.Kite{
		Username:    k.Config.Username,
		Environment: k.Config.Environment,
		Name:        k.name,
		Version:     k.version,
		Region:      k.Config.Region,
		Hostname:    hostname,
		ID:          k.Id,
	}
}

// KiteKey gives a kite key used to authenticate to kontrol and other kites.
func (k *Kite) KiteKey() string {
	k.configMu.RLock()
	defer k.configMu.RUnlock()

	return k.Config.KiteKey
}

// KontrolKey gives a Kontrol's public key.
//
// The value is taken form kite key's kontrolKey claim.
func (k *Kite) KontrolKey() *rsa.PublicKey {
	k.configMu.RLock()
	defer k.configMu.RUnlock()

	return k.kontrolKey
}

// HandleHTTP registers the HTTP handler for the given pattern into the
// underlying HTTP muxer.
func (k *Kite) HandleHTTP(pattern string, handler http.Handler) {
	k.muxer.Handle(pattern, handler)
}

// HandleHTTPFunc registers the HTTP handler for the given pattern into the
// underlying HTTP muxer.
func (k *Kite) HandleHTTPFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	k.muxer.HandleFunc(pattern, handler)
}

// ServeHTTP helps Kite to satisfy the http.Handler interface. So kite can be
// used as a standard http server.
func (k *Kite) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	k.muxer.ServeHTTP(w, req)
}

func (k *Kite) sockjsHandler(session sockjs.Session) {
	defer session.Close(3000, "Go away!")

	// This Client also handles the connected client.
	// Since both sides can send/receive messages the client code is reused here.
	c := k.NewClient("")

	c.setSession(session)
	c.wg.Add(1)
	go c.sendHub()

	k.callOnConnectHandlers(c)

	// Run after methods are registered and delegate is set
	c.readLoop()

	c.callOnDisconnectHandlers()
	k.callOnDisconnectHandlers(c)
}

// OnConnect registers a callbacks which is called when a Kite connects
// to the k Kite.
func (k *Kite) OnConnect(handler func(*Client)) {
	k.handlersMu.Lock()
	k.onConnectHandlers = append(k.onConnectHandlers, handler)
	k.handlersMu.Unlock()
}

// OnFirstRequest registers a function to run when we receive first request
// from other Kite.
func (k *Kite) OnFirstRequest(handler func(*Client)) {
	k.handlersMu.Lock()
	k.onFirstRequestHandlers = append(k.onFirstRequestHandlers, handler)
	k.handlersMu.Unlock()
}

// OnDisconnect registers a function to run when a connected Kite is disconnected.
func (k *Kite) OnDisconnect(handler func(*Client)) {
	k.handlersMu.Lock()
	k.onDisconnectHandlers = append(k.onDisconnectHandlers, handler)
	k.handlersMu.Unlock()
}

// OnRegister registers a callback which is called when a Kite registers
// to a Kontrol.
func (k *Kite) OnRegister(handler func(*protocol.RegisterResult)) {
	k.handlersMu.Lock()
	k.onRegisterHandlers = append(k.onRegisterHandlers, handler)
	k.handlersMu.Unlock()
}

func (k *Kite) callOnConnectHandlers(c *Client) {
	k.handlersMu.RLock()
	defer k.handlersMu.RUnlock()

	for _, handler := range k.onConnectHandlers {
		handler(c)
	}
}

func (k *Kite) callOnFirstRequestHandlers(c *Client) {
	k.handlersMu.RLock()
	defer k.handlersMu.RUnlock()

	for _, handler := range k.onFirstRequestHandlers {
		handler(c)
	}
}

func (k *Kite) callOnDisconnectHandlers(c *Client) {
	k.handlersMu.RLock()
	defer k.handlersMu.RUnlock()

	for _, handler := range k.onDisconnectHandlers {
		handler(c)
	}
}

func (k *Kite) callOnRegisterHandlers(r *protocol.RegisterResult) {
	k.handlersMu.RLock()
	defer k.handlersMu.RUnlock()

	for _, handler := range k.onRegisterHandlers {
		handler(r)
	}
}

func (k *Kite) updateAuth(reg *protocol.RegisterResult) {
	k.configMu.Lock()
	defer k.configMu.Unlock()

	switch {
	case reg.KiteKey != "":
		k.Config.KiteKey = reg.KiteKey

		ex := &kitekey.Extractor{
			Claims: &kitekey.KiteClaims{},
		}

		if _, err := jwt.ParseWithClaims(reg.KiteKey, ex.Claims, ex.Extract); err != nil {
			k.Log.Error("auth update: unable to extract kontrol key: %s", err)

			break
		}

		if ex.Claims.KontrolKey != "" {
			reg.PublicKey = ex.Claims.KontrolKey
		}
	}

	// we also received a new public key (means the old one was invalidated).
	// Use it now.
	if reg.PublicKey != "" {
		k.Config.KontrolKey = reg.PublicKey

		key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(reg.PublicKey))
		if err != nil {
			k.Log.Error("auth update: unable to update kontrol key: %s", err)

			return
		}

		k.kontrolKey = key
	}
}

// RSAKey returns the corresponding public key for the issuer of the token.
// It is called by jwt-go package when validating the signature in the token.
func (k *Kite) RSAKey(token *jwt.Token) (interface{}, error) {
	k.verifyOnce.Do(k.verifyInit)

	kontrolKey := k.KontrolKey()

	if kontrolKey == nil {
		panic("kontrol key is not set in config")
	}

	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, errors.New("invalid signing method")
	}

	claims, ok := token.Claims.(*kitekey.KiteClaims)
	if !ok {
		return nil, errors.New("token does not have valid claims")
	}

	if claims.Issuer != k.Config.KontrolUser {
		return nil, fmt.Errorf("issuer is not trusted: %s", claims.Issuer)
	}

	return kontrolKey, nil
}
