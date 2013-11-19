package kite

import (
	"flag"
	"fmt"
	logging "github.com/op/go-logging"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"time"
)

var log = logging.MustGetLogger("Kite")

// GetLogger returns a new logger which is used within the application itsel
// (in main package).
func GetLogger() *logging.Logger {
	return log
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

	// Registered is true if the Kite is registered to kontrol itself
	Registered bool

	// Points to the Kontrol instance if enabled
	Kontrol *Kontrol

	// Wheter we want to connect to Kontrol on startup, true by default.
	KontrolEnabled bool

	// Wheter we want to register our Kite to Kontrol, true by default.
	RegisterToKontrol bool

	// method map for exported methods
	handlers map[string]HandlerFunc

	// Dnode rpc server
	Server *rpc.Server

	// Contains different functions for authenticating user from request.
	// Keys are the authentication types (options.authentication.type).
	Authenticators map[string]func(*CallOptions) error
}

// New creates, initialize and then returns a new Kite instance. It accept
// three  arguments. options is a config struct that needs to be filled with
// several informations like Name, Port, IP and so on.
func New(options *protocol.Options) *Kite {
	var err error
	if options == nil {
		options, err = utils.ReadKiteOptions("manifest.json")
		if err != nil {
			log.Fatal("error: could not read config file", err)
		}
	}

	// some simple validations for config
	if options.Kitename == "" {
		log.Fatal("ERROR: options.Kitename field is not set")
	}

	if options.Region == "" {
		log.Fatal("ERROR: options.Region field is not set")
	}

	if options.Environment == "" {
		log.Fatal("ERROR: options.Environment field is not set")
	}

	hostname, _ := os.Hostname()
	kiteID := utils.GenerateUUID()

	kodingKey, err := utils.GetKodingKey()
	if err != nil {
		log.Fatal("Couldn't find koding.key. Please run 'kd register'.")
	}

	port := options.Port
	if options.Port == "" {
		port = "0" // OS binds to an automatic port
	}

	if options.KontrolAddr == "" {
		options.KontrolAddr = "127.0.0.1:4000" // local fallback address
	}

	k := &Kite{
		Kite: protocol.Kite{
			Name:        options.Kitename,
			Username:    options.Username,
			ID:          kiteID,
			Version:     options.Version,
			Hostname:    hostname,
			Port:        port,
			Environment: options.Environment,
			Region:      options.Region,

			// PublicIP will be set by Kontrol after registering if it is not set.
			PublicIP: options.PublicIP,
		},
		KodingKey:         kodingKey,
		Server:            rpc.NewServer(),
		KontrolEnabled:    true,
		RegisterToKontrol: true,
		Authenticators:    make(map[string]func(*CallOptions) error),
		handlers:          make(map[string]HandlerFunc),
	}
	k.Kontrol = k.NewKontrol(options.KontrolAddr)

	// Every kite should be able to authenticate the user from token.
	k.Authenticators["token"] = k.AuthenticateFromToken
	// A kite accepts requests from Kontrol.
	k.Authenticators["kodingKey"] = k.AuthenticateFromKodingKey

	// Register our internal methods
	k.HandleFunc("status", new(Status).Info)
	k.HandleFunc("heartbeat", k.handleHeartbeat)
	k.HandleFunc("log", k.handleLog)

	return k
}

func (k *Kite) HandleFunc(method string, handler HandlerFunc) {
	k.Server.HandleFunc(method, func(msg *dnode.Message, tr dnode.Transport) {
		request, responseCallback, err := k.parseRequest(msg, tr)
		if err != nil {
			log.Notice("Did not understand request: %s", err)
			return
		}

		result, err := handler(request)
		if responseCallback == nil {
			return
		}

		if err != nil {
			err = responseCallback(err.Error(), result)
		} else {
			err = responseCallback(nil, result)
		}

		if err != nil {
			log.Error(err.Error())
		}
	})
}

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously. It can be started in a goroutine if you wish to use kite as a
// client too.
func (k *Kite) Run() {
	k.parseVersionFlag()
	k.setupLogging()

	err := k.listenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func (k *Kite) handleHeartbeat(r *Request) (interface{}, error) {
	args, err := r.Args.Array()
	if err != nil {
		return nil, err
	}

	if len(args) != 2 {
		return nil, fmt.Errorf("Invalid args: %s", string(r.Args.Raw))
	}

	seconds, ok := args[0].(float64)
	if !ok {
		return nil, fmt.Errorf("Invalid interval: %s", args[0])
	}

	ping, ok := args[1].(dnode.Function)
	if !ok {
		return nil, fmt.Errorf("Invalid callback: %s", args[1])
	}

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
	s, err := r.Args.String()
	if err != nil {
		return nil, err
	}

	log.Info(fmt.Sprintf("%s: %s", r.RemoteKite.Name, s))
	return nil, nil
}

// setupLogging is used to setup the logging format, destination and level.
func (k *Kite) setupLogging() {
	log.Module = k.Name
	logging.SetFormatter(logging.MustStringFormatter("%{level:-8s} ▶ %{message}"))

	stderrBackend := logging.NewLogBackend(os.Stderr, "", stdlog.LstdFlags)
	stderrBackend.Color = true

	syslogBackend, _ := logging.NewSyslogBackend(k.Name)
	logging.SetBackend(stderrBackend, syslogBackend)

	// Set logging level. Default level is INFO.
	level := logging.INFO
	if k.hasDebugFlag() {
		level = logging.DEBUG
	}
	logging.SetLevel(level, log.Module)
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
	return false
}

// listenAndServe starts our rpc server with the given addr.
func (k *Kite) listenAndServe() error {
	listener, err := net.Listen("tcp4", ":"+k.Port)
	if err != nil {
		return err
	}

	log.Info("Listening: %s", listener.Addr().String())

	// Port is known here if "0" is used as port number
	_, k.Port, _ = net.SplitHostPort(listener.Addr().String())

	// We must connect to Kontrol after starting to listen on port
	if k.KontrolEnabled {
		if k.RegisterToKontrol {
			k.Kontrol.RemoteKite.Client.OnConnect(k.registerToKontrol)
		}

		k.Kontrol.DialForever()
	}

	return http.Serve(listener, k.Server)
}

func (k *Kite) registerToKontrol() {
	err := k.Kontrol.Register()
	if err != nil {
		log.Fatalf("Cannot register to Kontrol: %s", err)
	}

	log.Info("Registered to Kontrol successfully")
}

// DISABLED TEMPORARILY
// OnDisconnect adds the given function to the list of the users callback list
// which is called when the user is disconnected. There might be several
// connections from one user to the kite, in that case the functions are
// called only when all connections are closed.
// func (k *Kite) OnDisconnect(username string, f func()) {
// 	if addrs == nil {
// 		return
// 	}

// 	for _, addr := range addrs {
// 		client.onDisconnect = append(client.onDisconnect, f)
// 	}
// }
