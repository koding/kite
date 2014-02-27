// Package kite is a library for creating small micro-services.
// Two main types implemented by this package are
// Kite for creating a micro-service server called "Kite" and
// RemoteKite for communicating with another kites.
package kite

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode/rpc"
	"github.com/koding/kite/logging"
	"github.com/koding/kite/protocol"
	"github.com/nu7hatch/gouuid"
)

var hostname string

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("kite: cannot get hostname: %s", err.Error()))
	}

	// Debugging helper: Prints stacktrace on SIGUSR1.
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for {
			s := <-c
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
	onConnectHandlers []func(*RemoteKite)

	// Handlers to call when a client has disconnected.
	onDisconnectHandlers []func(*RemoteKite)

	name    string
	version string
	id      string // Unique kite instance id
}

// New creates, initialize and then returns a new Kite instance. It accepts
// a single options argument that is a config struct that needs to be filled
// with several informations like Name, Port, IP and so on.
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

	// Every kite should be able to authenticate the user from token.
	// Tokens are granted by Kontrol Kite.
	k.Authenticators["token"] = k.AuthenticateFromToken

	// A kite accepts requests with the same username.
	k.Authenticators["kiteKey"] = k.AuthenticateFromKiteKey

	// Register default methods.
	k.addDefaultHandlers()

	return k
}

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
	case "INFO":
		logger.SetLevel(logging.INFO)
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

// OnConnect registers a function to run when a Kite connects to this Kite.
func (k *Kite) OnConnect(handler func(*RemoteKite)) {
	k.onConnectHandlers = append(k.onConnectHandlers, handler)
}

// OnDisconnect registers a function to run when a connected Kite is disconnected.
func (k *Kite) OnDisconnect(handler func(*RemoteKite)) {
	k.onDisconnectHandlers = append(k.onDisconnectHandlers, handler)
}

// notifyRemoteKiteConnected runs the registered handlers with OnConnect().
func (k *Kite) notifyRemoteKiteConnected(r *RemoteKite) {
	k.Log.Info("Client '%s' is identified as '%s'",
		r.client.Conn.Request().RemoteAddr, r.Name)

	for _, handler := range k.onConnectHandlers {
		handler(r)
	}
}

// notifyRemoteKiteDisconnected runs the registered handlers with OnDisconnect().
func (k *Kite) notifyRemoteKiteDisconnected(r *RemoteKite) {
	k.Log.Info("Client has disconnected: %s", r.Name)

	for _, handler := range k.onDisconnectHandlers {
		handler(r)
	}
}
