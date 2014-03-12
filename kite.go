// Package kite is a library for creating small micro-services.
// Two main types implemented by this package are
// Kite for creating a micro-service server called "Kite" and
// Client for communicating with another kites.
package kite

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode/rpc"
	"github.com/koding/kite/protocol"
	"github.com/koding/logging"
	"github.com/nu7hatch/gouuid"
)

var hostname string

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("kite: cannot get hostname: %s", err.Error()))
	}
}

// Kite defines a single process that enables distributed service messaging
// amongst the peers it is connected. A Kite process acts as a Client and as a
// Server. That means it can receive request, process them, but it also can
// make request to other kites.
//
// Do not use this struct directly. Use kite.New function, add your handlers
// with HandleFunc mehtod, then call Start or Run method.
type Kite struct {
	Config *config.Config

	// Prints logging messages to stderr.
	Log logging.Logger

	// Contains different functions for authenticating user from request.
	// Keys are the authentication types (options.authentication.type).
	Authenticators map[string]func(*Request) error

	// Kontrol keys to trust. Kontrol will issue access tokens for kites
	// that are signed with the private counterpart of these keys.
	// Key data must be PEM encoded.
	trustedKontrolKeys map[string]string

	handlers map[string]HandlerFunc // method map for exported methods
	server   *rpc.Server            // Dnode rpc server

	// Handlers to call when a Kite opens a connection to this Kite.
	onConnectHandlers []func(*Client)

	// Handlers to call when a client has disconnected.
	onDisconnectHandlers []func(*Client)

	name    string
	version string
	id      string // Unique kite instance id
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

	kiteID, err := uuid.NewV4()
	if err != nil {
		panic(fmt.Sprintf("kite: cannot generate unique ID: %s", err.Error()))
	}

	k := &Kite{
		Config:             config.New(),
		Log:                newLogger(name),
		Authenticators:     make(map[string]func(*Request) error),
		server:             rpc.NewServer(),
		handlers:           make(map[string]HandlerFunc),
		trustedKontrolKeys: make(map[string]string),
		name:               name,
		version:            version,
		id:                 kiteID.String(),
	}

	// Wrap/unwrap dnode messages.
	k.server.SetWrappers(wrapMethodArgs, wrapCallbackArgs, runMethod, runCallback, onError)

	// Server needs a reference to local kite
	k.server.Properties()["localKite"] = k

	k.server.OnConnect(func(c *rpc.Client) {
		k.Log.Info("New connection from: %s", c.RemoteAddr())
	})

	// Call registered handlers when a client has disconnected.
	k.server.OnDisconnect(func(c *rpc.Client) {
		if r, ok := c.Properties()["client"]; ok {
			// Run OnDisconnect handlers.
			k.notifyClientDisconnected(r.(*Client))
		}
	})

	// Every kite should be able to authenticate the user from token.
	// Tokens are granted by Kontrol Kite.
	k.Authenticators["token"] = k.AuthenticateFromToken

	// A kite accepts requests with the same username.
	k.Authenticators["kiteKey"] = k.AuthenticateFromKiteKey

	// Register default methods.
	k.addDefaultHandlers()

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
		ID:          k.id,
	}
}

// Trust a Kontrol key for validating tokens.
func (k *Kite) TrustKontrolKey(issuer, key string) {
	k.trustedKontrolKeys[issuer] = key
}

func (k *Kite) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	k.server.ServeHTTP(w, req)
}

// newLogger returns a new logger object for desired name and level.
func newLogger(name string) logging.Logger {
	logger := logging.NewLogger(name)

	switch strings.ToUpper(os.Getenv("KITE_LOG_LEVEL")) {
	case "DEBUG":
		logger.SetLevel(logging.DEBUG)
	case "NOTICE":
		logger.SetLevel(logging.NOTICE)
	case "WARNING":
		logger.SetLevel(logging.WARNING)
	case "ERROR":
		logger.SetLevel(logging.ERROR)
	case "CRITICAL":
		logger.SetLevel(logging.CRITICAL)
	default:
		logger.SetLevel(logging.INFO)
	}

	return logger
}

// OnFirstRequest registers a function to run when a Kite connects to this Kite.
func (k *Kite) OnFirstRequest(handler func(*Client)) {
	k.onConnectHandlers = append(k.onConnectHandlers, handler)
}

// OnDisconnect registers a function to run when a connected Kite is disconnected.
func (k *Kite) OnDisconnect(handler func(*Client)) {
	k.onDisconnectHandlers = append(k.onDisconnectHandlers, handler)
}

// notifyFirstRequest runs the registered handlers with OnFirstRequest().
func (k *Kite) notifyFirstRequest(r *Client) {
	k.Log.Info("Client '%s' is identified as '%s'",
		r.client.RemoteAddr(), r.Kite)

	for _, handler := range k.onConnectHandlers {
		handler(r)
	}
}

// notifyClientDisconnected runs the registered handlers with OnDisconnect().
func (k *Kite) notifyClientDisconnected(r *Client) {
	k.Log.Info("Client has disconnected: %s", r.Kite)

	for _, handler := range k.onDisconnectHandlers {
		handler(r)
	}
}

// RSAKey returns the corresponding public key for the issuer of the token.
// It is called by jwt-go package when validating the signature in the token.
func (k *Kite) RSAKey(token *jwt.Token) ([]byte, error) {
	if k.Config.KontrolKey == "" {
		panic("kontrol key is not set in config")
	}

	issuer, ok := token.Claims["iss"].(string)
	if !ok {
		return nil, errors.New("token does not contain a valid issuer claim")
	}

	if issuer != k.Config.KontrolUser {
		return nil, fmt.Errorf("issuer is not trusted: %s", issuer)
	}

	return []byte(k.Config.KontrolKey), nil
}

// Normally, each incoming request is processed in a new goroutine.
// If you disable concurrency, requests will be processed synchronously.
func (k *Kite) DisableConcurrency() {
	k.server.SetConcurrent(false)
}

// SetupSignalHandler listens to SIGUSR1 signal and prints a stackrace for every
// SIGUSR1 signal
func (k *Kite) SetupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for s := range c {
			fmt.Println("Got signal:", s)
			buf := make([]byte, 1<<16)
			runtime.Stack(buf, true)
			fmt.Println(string(buf))
			fmt.Print("Number of goroutines:", runtime.NumGoroutine())
			m := new(runtime.MemStats)
			runtime.GC()
			runtime.ReadMemStats(m)
			fmt.Printf(", Memory allocated: %+v\n", m.Alloc)
		}
	}()
}
