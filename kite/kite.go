package kite

import (
	"crypto/tls"
	"flag"
	"fmt"
	"koding/newkite/dnode/rpc"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

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
// make request to other kites. A Kite can be anything. It can be simple Image
// processing kite (which would process data), it could be a Chat kite that
// enables peer-to-peer chat. For examples we have FileSystem kite that expose
// the file system to a client, which in order build the filetree.
type Kite struct {
	protocol.Kite

	// KodingKey is used for authenticate to Kontrol.
	KodingKey string

	// Is this Kite Public or Private? Default is Private.
	Visibility protocol.Visibility

	// Points to the Kontrol instance if enabled
	Kontrol *Kontrol

	// Wheter we want to connect to Kontrol on startup, true by default.
	KontrolEnabled bool

	// Wheter we want to register our Kite to Kontrol, true by default.
	RegisterToKontrol bool

	// Use Koding.com's TLS reverse-proxy server for incoming connections.
	// Instead of the Kite's address, address of the TLS proxy will be
	// registered to Kontrol.
	tlsProxyEnabled bool

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

	hostname, _ := os.Hostname()
	kiteID := utils.GenerateUUID()

	// Enable authentication. options.DisableAuthentication is false by
	// default due to Go's varible initialization.
	var kodingKey string
	if !options.DisableAuthentication {
		kodingKey, err = utils.GetKodingKey()
		if err != nil {
			log.Fatal("Couldn't find koding.key. Please run 'kd register'.")
		}
	}

	k := &Kite{
		Kite: protocol.Kite{
			Name:        options.Kitename,
			Username:    options.Username,
			ID:          kiteID,
			Version:     options.Version,
			Hostname:    hostname,
			Environment: options.Environment,
			Region:      options.Region,
			Visibility:  options.Visibility,
			URL: protocol.KiteURL{
				&url.URL{
					Scheme: "ws",
					Host:   net.JoinHostPort(options.PublicIP, options.Port),
					Path:   "/dnode",
				},
			},
		},
		KodingKey:           kodingKey,
		server:              rpc.NewServer(),
		concurrent:          true,
		KontrolEnabled:      true,
		RegisterToKontrol:   true,
		Authenticators:      make(map[string]func(*Request) error),
		disableAuthenticate: options.DisableAuthentication,
		handlers:            make(map[string]HandlerFunc),
		ready:               make(chan bool),
		end:                 make(chan bool, 1),
	}

	k.server.SetWrappers(wrapMethodArgs, wrapCallbackArgs, runMethod, runCallback, onError)
	k.server.Properties()["localKite"] = k

	k.Log = newLogger(k.Name, k.hasDebugFlag())
	k.Kontrol = k.NewKontrol(options.KontrolURL)

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
	k.Authenticators["kodingKey"] = k.AuthenticateFromKodingKey

	// Register our internal methods
	k.HandleFunc("systemInfo", new(Status).Info)
	k.HandleFunc("heartbeat", k.handleHeartbeat)
	k.HandleFunc("log", k.handleLog)

	return k
}

func (k *Kite) DisableConcurrency() {
	k.server.SetConcurrent(false)
}

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

func (k *Kite) EnableTLSProxy() {
	k.tlsProxyEnabled = true
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
func newLogger(name string, debug bool) *logging.Logger {
	logger := logging.MustGetLogger(name)

	level := logging.INFO
	if debug {
		level = logging.DEBUG
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

// hasDebugFlag returns true if -debug flag is present in os.Args.
func (k *Kite) hasDebugFlag() bool {
	for _, flag := range os.Args {
		if flag == "-debug" {
			return true
		}
	}

	// We can't use flags when running "go test" command.
	// This is another way to print debug logs.
	if os.Getenv("DEBUG") != "" {
		return true
	}

	return false
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

	registerURLs := make(chan *url.URL, 1)

	if k.tlsProxyEnabled {
		// Register to TLS Kite and stay connected.
		// Fill the channel with registered TLS URLs.
		go k.registerToTLS(registerURLs)
	} else {
		// Register with Kite's own URL.
		registerURLs <- k.URL.URL
	}

	// We must connect to Kontrol after starting to listen on port
	if k.KontrolEnabled {
		if err = k.Kontrol.DialForever(); err != nil {
			return
		}

		if k.RegisterToKontrol {
			go k.registerToKontrol(registerURLs)
		}
	}

	k.ready <- true // listener is ready, unblock Start().

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	err = http.Serve(k.listener, k.server)
	if strings.Contains(err.Error(), errClosing) {
		// The server is closed by Close() method
		err = nil
	}

	k.end <- true // Serving is finished.

	return err
}

func (k *Kite) registerToKontrol(urls chan *url.URL) {
	for k.URL.URL = range urls {
		for {
			err := k.Kontrol.Register()
			if err != nil {
				k.Log.Fatalf("Cannot register to Kontrol: %s", err)
				time.Sleep(60 * time.Second)
			}

			// Registered to Kontrol.
			break
		}
	}
}

func (k *Kite) registerToTLS(urls chan *url.URL) {
	query := protocol.KontrolQuery{
		Username:    "devrim",
		Environment: "production",
		Name:        "tls",
	}

	for {
		kites, err := k.Kontrol.GetKites(query)
		if err != nil {
			k.Log.Error("Cannot get TLS kites from Kontrol: %s", err.Error())
			time.Sleep(1)
			continue
		}

		tls := kites[rand.Int()%len(kites)]

		// Notify us on disconnect
		disconnect := make(chan bool, 1)
		tls.OnDisconnect(func() {
			select {
			case disconnect <- true:
			default:
			}
		})

		err = tls.Dial()
		if err != nil {
			k.Log.Error("Cannot connect to TLS kite: %s", err.Error())
			time.Sleep(1)
			continue
		}

		result, err := tls.Tell("register")
		if err != nil {
			k.Log.Error("TLS register error: %s", err.Error())
			tls.Close()
			time.Sleep(1)
			continue
		}

		tlsURL, err := result.String()
		if err != nil {
			k.Log.Error("TLS register result error: %s", err.Error())
			tls.Close()
			time.Sleep(1)
			continue
		}

		parsed, err := url.Parse(tlsURL)
		if err != nil {
			k.Log.Error("Cannot parse TLS URL: %s", err.Error())
			tls.Close()
			time.Sleep(1)
			continue
		}

		if k.KontrolEnabled && k.RegisterToKontrol {
			urls <- parsed
		}

		// Block until disconnect from TLS Kite.
		<-disconnect
	}
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
	k.Log.Info("Client is connected to us: %s", r.Name)

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
