// Package kite is a library for creating small micro-services.
// Two main types implemented by this package are
// Kite for creating a micro-service server called "Kite" and
// RemoteKite for communicating with another kites.
package kite

import (
	"crypto/tls"
	"flag"
	"fmt"
	"kite/dnode/rpc"
	"kite/kitekey"
	"kite/protocol"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
	"github.com/op/go-logging"
)

func init() {
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
	protocol.Kite

	// Is this Kite Public or Private? Default is Private.
	Visibility protocol.Visibility

	// Points to the Kontrol instance if enabled
	Kontrol *Kontrol

	// Parsed JWT token from ~/.kite/kite.key
	kiteKey *jwt.Token

	// Wheter we want to connect to Kontrol on startup, true by default.
	KontrolEnabled bool

	// Wheter we want to register our Kite to Kontrol, true by default.
	RegisterToKontrol bool

	// Use Koding.com's reverse-proxy server for incoming connections.
	// Instead of the Kite's address, address of the Proxy Kite will be
	// registered to Kontrol.
	proxyEnabled bool

	// method map for exported methods
	handlers map[string]HandlerFunc

	// Should handlers run concurrently? Default is true.
	concurrent bool

	// Dnode rpc server
	server *rpc.Server

	listener net.Listener

	// Handlers to call when a Kite opens a connection to this Kite.
	onConnectHandlers []func(*RemoteKite)

	// Handlers to call when a client has disconnected.
	onDisconnectHandlers []func(*RemoteKite)

	// Contains different functions for authenticating user from request.
	// Keys are the authentication types (options.authentication.type).
	Authenticators map[string]func(*Request) error

	// Should kite disable authenticators for incoming requests? Disabled by default
	disableAuthenticate bool

	// Kontrol keys to trust. Kontrol will issue access tokens for kites
	// that are signed with the private counterpart of these keys.
	// Key data must be PEM encoded.
	trustedKontrolKeys map[string]string

	// Trusted root certificates for TLS connections (wss://).
	// Certificate data must be PEM encoded.
	tlsCertificates [][]byte

	// Used to signal if the kite is ready to start and make calls to
	// other kites.
	ready chan bool
	end   chan bool

	// Prints logging messages to stderr and syslog.
	Log *logging.Logger
}

// New creates, initialize and then returns a new Kite instance. It accepts
// a single options argument that is a config struct that needs to be filled
// with several informations like Name, Port, IP and so on.
func New(options *Options) *Kite {
	var err error

	if options == nil {
		options, err = ReadKiteOptions("manifest.json")
		if err != nil {
			log.Fatal("error: could not read config file", err)
		}
	}

	options.validate() // exits if validating fails

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	kiteID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}

	k := &Kite{
		Kite: protocol.Kite{
			Name:        options.Kitename,
			ID:          kiteID.String(),
			Version:     options.Version,
			Hostname:    hostname,
			Environment: options.Environment,
			Region:      options.Region,
			Visibility:  options.Visibility,
			URL: protocol.KiteURL{
				&url.URL{
					Scheme: "ws",
					Host:   net.JoinHostPort(options.PublicIP, options.Port),
					Path:   options.Path,
				},
			},
		},
		server:              rpc.NewServer(),
		concurrent:          true,
		KontrolEnabled:      true,
		RegisterToKontrol:   true,
		trustedKontrolKeys:  make(map[string]string),
		Authenticators:      make(map[string]func(*Request) error),
		disableAuthenticate: options.DisableAuthentication,
		handlers:            make(map[string]HandlerFunc),
		ready:               make(chan bool),
		end:                 make(chan bool, 1),
	}

	k.Log = newLogger(k.Name)

	k.kiteKey, err = kitekey.Parse()
	if err != nil {
		k.Log.Warning("Cannot read kite key. You must register by running \"kite register\" command.")
	} else if !k.kiteKey.Valid {
		panic(err)
	} else {
		k.Username = k.kiteKey.Claims["sub"].(string)

		if k.KontrolEnabled {
			parsedURL, err := url.Parse(k.kiteKey.Claims["kontrolURL"].(string))
			if err != nil {
				panic(err)
			}

			k.Kontrol = k.NewKontrol(parsedURL)

			k.TrustKontrolKey(k.kiteKey.Claims["iss"].(string), k.kiteKey.Claims["kontrolKey"].(string))
		}
	}

	k.server.SetWrappers(wrapMethodArgs, wrapCallbackArgs, runMethod, runCallback, onError)
	k.server.Properties()["localKite"] = k

	k.server.OnConnect(func(c *rpc.Client) {
		k.Log.Info("New connection from: %s", c.Conn.Request().RemoteAddr)
	})

	// Call registered handlers when a client has disconnected.
	k.server.OnDisconnect(func(c *rpc.Client) {
		if r, ok := c.Properties()["remoteKite"]; ok {
			// Run OnDisconnect handlers.
			k.notifyRemoteKiteDisconnected(r.(*RemoteKite))
		}
	})

	// Every kite should be able to authenticate the user from token.
	k.Authenticators["token"] = k.AuthenticateFromToken
	// A kite accepts requests from Kontrol.
	k.Authenticators["kiteKey"] = k.AuthenticateFromKiteKey

	// Register our internal methods
	k.HandleFunc("systemInfo", new(status).Info)
	k.HandleFunc("heartbeat", k.handleHeartbeat)
	k.HandleFunc("log", k.handleLog)

	return k
}

// Normally, each incoming request is processed in a new goroutine.
// If you disable concurrency, requests will be processed synchronously.
func (k *Kite) DisableConcurrency() {
	k.server.SetConcurrent(false)
}

// EnableTLS enables "wss://" protocol".
// It uses the same port and disables "ws://".
func (k *Kite) EnableTLS(certFile, keyFile string) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		k.Log.Fatal(err.Error())
	}

	k.server.TlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	k.Kite.URL.Scheme = "wss"
}

// Put this kite behind a reverse-proxy. Useful under firewall or NAT.
func (k *Kite) EnableProxy() {
	k.proxyEnabled = true
}

// Trust a Kontrol key for validating tokens.
func (k *Kite) TrustKontrolKey(issuer, key string) {
	k.trustedKontrolKeys[issuer] = key
}

// Add new trusted root certificate for TLS.
func (k *Kite) AddRootCertificate(cert []byte) {
	k.tlsCertificates = append(k.tlsCertificates, cert)
}

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously.
func (k *Kite) Run() {
	k.Start()
	<-k.end
	k.Log.Notice("Kite server is closed.")
}

// Start is like Run(), but does not wait for it to complete. It's nonblocking.
func (k *Kite) Start() {
	k.parseVersionFlag()

	go func() {
		err := k.listenAndServe()
		if err != nil {
			k.Log.Fatal(err)
		}
	}()

	<-k.ready // wait until we are ready
}

// Close stops the server.
func (k *Kite) Close() {
	k.Log.Notice("Closing server...")
	k.listener.Close()
}

func (k *Kite) handleHeartbeat(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	seconds := args[0].MustFloat64()
	ping := args[1].MustFunction()

	go func() {
		for {
			time.Sleep(time.Duration(seconds) * time.Second)
			if ping() != nil {
				return
			}
		}
	}()

	return nil, nil
}

// handleLog prints a log message to stdout.
func (k *Kite) handleLog(r *Request) (interface{}, error) {
	msg := r.Args.One().MustString()
	k.Log.Info(fmt.Sprintf("%s: %s", r.RemoteKite.Name, msg))
	return nil, nil
}

func init() {
	// These logging related stuff needs to be called once because stupid
	// logging library uses global variables and resets the backends every time.
	logging.SetFormatter(logging.MustStringFormatter("%{level:-8s} ▶ %{message}"))
	stderrBackend := logging.NewLogBackend(os.Stderr, "", log.LstdFlags)
	stderrBackend.Color = true
	syslogBackend, _ := logging.NewSyslogBackend("")
	logging.SetBackend(stderrBackend, syslogBackend)
}

// newLogger returns a new logger object for desired name and level.
func newLogger(name string) *logging.Logger {
	logger := logging.MustGetLogger(name)

	var level logging.Level
	switch strings.ToUpper(os.Getenv("KITE_LOG_LEVEL")) {
	case "DEBUG":
		level = logging.DEBUG
	case "INFO":
		level = logging.INFO
	case "NOTICE":
		level = logging.NOTICE
	case "WARNING":
		level = logging.WARNING
	case "ERROR":
		level = logging.ERROR
	case "CRITICAL":
		level = logging.CRITICAL
	default:
		level = logging.INFO
	}

	logging.SetLevel(level, name)
	return logger
}

// If the user wants to call flag.Parse() the flag must be defined in advance.
var _ = flag.Bool("version", false, "show version")
var _ = flag.Bool("debug", false, "print debug logs")

// parseVersionFlag prints the version number of the kite and exits with 0
// if "-version" flag is enabled.
// We did not use the "flag" package because it causes trouble if the user
// also calls "flag.Parse()" in his code. flag.Parse() can be called only once.
func (k *Kite) parseVersionFlag() {
	for _, flag := range os.Args {
		if flag == "-version" {
			fmt.Println(k.Version)
			os.Exit(0)
		}
	}
}

// listenAndServe starts our rpc server with the given addr.
func (k *Kite) listenAndServe() (err error) {
	k.listener, err = net.Listen("tcp4", k.Kite.URL.Host)
	if err != nil {
		return err
	}

	k.Log.Notice("Listening: %s", k.listener.Addr().String())

	// Enable TLS
	if k.server.TlsConfig != nil {
		k.listener = tls.NewListener(k.listener, k.server.TlsConfig)
	}

	// Port is known here if "0" is used as port number
	host, _, _ := net.SplitHostPort(k.Kite.URL.Host)
	_, port, _ := net.SplitHostPort(k.listener.Addr().String())
	k.Kite.URL.Host = net.JoinHostPort(host, port)

	// Kontrol will register to the URLs sent to this channel.
	registerURLs := make(chan *url.URL, 1)

	if k.proxyEnabled {
		// Register to Proxy Kite and stay connected.
		// Fill the channel with registered Proxy URLs.
		go k.keepRegisteredToProxyKite(registerURLs)
	} else {
		// If proxy is not enabled, we must populate the channel ourselves.
		registerURLs <- k.URL.URL // Register with Kite's own URL.
	}

	// We must connect to Kontrol after starting to listen on port
	if k.KontrolEnabled && k.Kontrol != nil {
		if err = k.Kontrol.DialForever(); err != nil {
			k.Log.Critical(err.Error())
		}

		if k.RegisterToKontrol {
			go k.keepRegisteredToKontrol(registerURLs)
		}
	}

	k.ready <- true // listener is ready, unblock Start().

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	k.Log.Notice("Serving on: %s", k.URL.URL.String())
	err = http.Serve(k.listener, k.server)
	if strings.Contains(err.Error(), errClosing) {
		// The server is closed by Close() method
		err = nil
	}

	k.end <- true // Serving is finished.

	return err
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
		go handler(r)
	}
}

func (k *Kite) notifyRemoteKiteDisconnected(r *RemoteKite) {
	k.Log.Info("Client has disconnected: %s", r.Name)

	for _, handler := range k.onDisconnectHandlers {
		go handler(r)
	}
}
