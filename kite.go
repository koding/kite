// Package kite is a library for creating small micro-services.
// Two main types implemented by this package are
// Kite for creating a micro-service server called "Kite" and
// RemoteKite for communicating with another kites.
package kite

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"github.com/koding/kite/dnode/rpc"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/logging"
	"github.com/koding/kite/protocol"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
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

	// will be used in query to kontrol for finding proxy kite
	proxyUsername string

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

	// To signal when kite is ready to accept connections
	readyC chan bool

	// To signal when kite is closed with Close()
	closeC chan bool

	// Prints logging messages to stderr and syslog.
	Log logging.Logger

	// Original URL that the kite server is serving on.
	ServingURL *url.URL
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
			// Username will be set after reading the kite.key
			Name:        options.Kitename,
			ID:          kiteID.String(),
			Version:     options.Version,
			Hostname:    hostname,
			Environment: options.Environment,
			Region:      options.Region,
			URL: &protocol.KiteURL{
				url.URL{
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
		readyC:              make(chan bool),
		closeC:              make(chan bool),
	}
	tempURL := k.Kite.URL.URL
	k.ServingURL = &tempURL

	k.Log = newLogger(k.Name)

	k.kiteKey, err = kitekey.Parse()
	if err != nil {
		if k.Name != "kite-command" && k.Name != "regserv" {
			k.Log.Warning("Cannot read kite key. You must register by running \"kite register\" command.")
		}
	} else if !k.kiteKey.Valid {
		panic(err)
	} else {
		k.Username = k.kiteKey.Claims["sub"].(string)

		if k.KontrolEnabled {
			kontrolURL := os.Getenv("KITE_KONTROL_URL")
			if kontrolURL == "" {
				kontrolURL = k.kiteKey.Claims["kontrolURL"].(string)
			}

			parsedURL, err := url.Parse(kontrolURL)
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

	// Register default methods.
	k.HandleFunc("systemInfo", systemInfo)
	k.HandleFunc("heartbeat", k.handleHeartbeat)
	k.HandleFunc("tunnel", handleTunnel)
	k.HandleFunc("log", k.handleLog)
	k.HandleFunc("print", handlePrint)
	k.HandleFunc("prompt", handlePrompt)
	if runtime.GOOS == "darwin" {
		k.HandleFunc("notify", handleNotifyDarwin)
	}

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
	k.ServingURL.Scheme = "wss"
}

// Put this kite behind a reverse-proxy. Useful under firewall or NAT.
func (k *Kite) EnableProxy(username string) {
	k.proxyEnabled = true
	k.proxyUsername = username
}

// Trust a Kontrol key for validating tokens.
func (k *Kite) TrustKontrolKey(issuer, key string) {
	k.trustedKontrolKeys[issuer] = key
}

// Add new trusted root certificate for TLS from a PEM block.
func (k *Kite) AddRootCertificate(cert string) {
	k.tlsCertificates = append(k.tlsCertificates, []byte(cert))
}

// Add new trusted root certificate for TLS from a file name.
func (k *Kite) AddRootCertificateFile(certFile string) {
	data, err := ioutil.ReadFile(certFile)
	if err != nil {
		k.Log.Fatal("Cannot add certificate: %s", err.Error())
	}
	k.tlsCertificates = append(k.tlsCertificates, data)
}

func (k *Kite) CloseNotify() chan bool {
	return k.closeC
}

func (k *Kite) ReadyNotify() chan bool {
	return k.readyC
}

// Start is like Run(), but does not wait for it to complete. It's nonblocking.
func (k *Kite) Start() {
	go k.Run()
	<-k.readyC // wait until we are ready
}

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously.
func (k *Kite) Run() {
	if os.Getenv("KITE_VERSION") != "" {
		fmt.Println(k.Version)
		os.Exit(0)
	}

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	err := k.ListenAndServe()
	if err != nil {
		if strings.Contains(err.Error(), errClosing) {
			// The server is closed by Close() method
			k.Log.Notice("Kite server is closed.")
			return
		}
		k.Log.Fatal(err.Error())
	}
}

// Close stops the server.
func (k *Kite) Close() {
	k.Log.Notice("Closing server...")
	k.listener.Close()
	k.Log.Close()
}

// ListenAndServe listens on the TCP network address k.URL.Host and then
// calls Serve to handle requests on incoming connections.
func (k *Kite) ListenAndServe() error {
	var err error

	k.listener, err = net.Listen("tcp4", k.Kite.URL.Host)
	if err != nil {
		return err
	}

	k.Log.Notice("Listening: %s", k.listener.Addr().String())

	// Enable TLS
	if k.server.TlsConfig != nil {
		k.listener = tls.NewListener(k.listener, k.server.TlsConfig)
	}

	return k.Serve(k.listener)
}

func (k *Kite) Serve(l net.Listener) error {
	// Must register to proxy and/or kontrol before start serving.
	// Otherwise, no one can find us.
	k.Register(l.Addr())

	k.Log.Notice("Serving on: %s", k.URL.URL.String())

	close(k.readyC)       // listener is ready, notify waiters.
	defer close(k.closeC) // serving is finished, notify waiters.

	return http.Serve(l, k)
}

// Register to proxy and/or kontrol, then update the URL.
func (k *Kite) Register(addr net.Addr) {
	// Port is known here if "0" is used as port number
	host, _, _ := net.SplitHostPort(k.Kite.URL.Host)
	_, port, _ := net.SplitHostPort(addr.String())
	k.Kite.URL.Host = net.JoinHostPort(host, port)
	k.ServingURL.Host = k.Kite.URL.Host

	// Kontrol will register to the URLs sent to this channel.
	registerURLs := make(chan *url.URL, 1)

	if k.proxyEnabled {
		// Register to Proxy Kite and stay connected.
		// Fill the channel with registered Proxy URLs.
		go k.keepRegisteredToProxyKite(registerURLs)
	} else {
		// If proxy is not enabled, we must populate the channel ourselves.
		registerURLs <- &k.URL.URL // Register with Kite's own URL.
	}

	// We must connect to Kontrol after starting to listen on port
	if k.KontrolEnabled && k.Kontrol != nil {
		if err := k.Kontrol.DialForever(); err != nil {
			k.Log.Critical(err.Error())
		}

		if k.RegisterToKontrol {
			go k.keepRegisteredToKontrol(registerURLs)
		}
	}
}

func (k *Kite) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	k.server.ServeHTTP(w, req)
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

// notifyRemoteKiteDisconnected runs the registered handlers with OnDisconnect().
func (k *Kite) notifyRemoteKiteDisconnected(r *RemoteKite) {
	k.Log.Info("Client has disconnected: %s", r.Name)

	for _, handler := range k.onDisconnectHandlers {
		go handler(r)
	}
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
