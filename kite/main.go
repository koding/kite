package kite

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fatih/goset"
	"github.com/golang/groupcache"
	"io"
	"koding/db/models"
	"koding/newkite/peers"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"koding/tools/slog"
	"math"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"reflect"
	"sync"
	"time"
)

var (
	// in-memory hash table for kites of same types
	kites = peers.New()

	// roundrobin load balancing helpers
	balance = NewBalancer()

	// set data structure for caching tokens
	permissions = goset.New()
)

// Messenger is used to implement various Messaging patterns on top of the
// Kites.
type Messenger interface {
	// Send is makes a request to the endpoint and returns the response
	Send([]byte) []byte

	// Consumer is a subscriber/consumer that listens to the endpoint. Incoming
	// data should be handler via the function that is passed.
	Consume(func([]byte))

	// To subscribe to a certain topic
	Subscribe(string) error

	// Unsubscribe from a certain topic
	Unsubscribe(string) error
}

// Clients is an interface that encapsulates basic operations on incoming and
// connected clients.
type Clients interface {
	// Add inserts a new client into the storage.
	Add(c *client)

	// Get returns a new client that matches the c.Addr field
	Get(c *client) *client

	// Remove deletes the client that matches the c.Addr field
	Remove(c *client)

	// Size returns the total number of clients connected currently
	Size() int

	// List returns a slice of all clients
	List() []*client
}

// Kite defines a single process that enables distributed service messaging
// amongst the peers it is connected. A Kite process acts as a Client and as a
// Server. That means it can receive request, process them, but it also can
// make request to other kites. A Kite can be anything. It can be simple Image
// processing kite (which would process data), it could be a Chat kite that
// enables peer-to-peer chat. For examples we have FileSystem kite that expose
// the file system to a client, which in order build the filetree.
type Kite struct {
	// User that calls/runs the kite
	Username string

	// Kitename defines the name that a kite is running on. This field is also
	// used for communicating with other kites with the same name.
	Kitename string

	// Uuid is a genereated unique id string that defines this Kite.
	Uuid string

	// RPC and GroupCache addresses, also expoxed to Kontrol
	Addr string

	// PublicKey is used for authenticate to Kontrol.
	PublicKey string

	// Hostname the kite is running on. Uses os.Hostname()
	Hostname string

	LocalIP  string // local network interface
	PublicIP string // public reachable IP

	// Port that the kite is going to be run.
	Port string

	// every kite should have version
	Version string

	// Registered is true if the Kite is registered to kontrol itself
	Registered bool

	// other kites that needs to be run, in order to run this one
	Dependencies string

	// by default yes, if disabled it bypasses kontrol
	KontrolEnabled bool

	// method map for shared methods
	Methods map[string]string

	// implements the Messenger interface
	Messenger Messenger

	// implements the Clients interface
	Clients Clients

	// GroupCache variables
	Pool  *groupcache.HTTPPool
	Group *groupcache.Group

	// RpcServer
	Server *rpc.Server

	// used to start the rpc server only once
	OnceServer sync.Once

	// used when multiple goroutines are requesting information from kontrol
	// we only make on request to Kontrol.
	OnceCall sync.Once
}

// New creates, initialize and then returns a new Kite instance. It accept
// three  arguments. options is a config struct that needs to be filled with
// several informations like Name, Port, IP and so on.
func New(options *protocol.Options) *Kite {
	var err error
	if options == nil {
		options, err = utils.ReadKiteOptions("manifest.json")
		if err != nil {
			slog.Fatal("error: could not read config file", err)
		}
	}

	// some simple validations for config
	if options.Kitename == "" {
		slog.Fatal("error: options data is not set properly")
	}

	// define log settings
	slog.SetPrefixName(options.Kitename)
	slog.SetPrefixTimeStamp(time.Kitchen) // let it be simple

	hostname, _ := os.Hostname()
	kiteID := utils.GenerateUUID()

	publicKey, err := utils.GetKodingKey("public")
	if err != nil {
		slog.Fatal("public key reading:", err)
	}

	publicIP := utils.GetPublicIP(options.PublicIP)
	localIP := utils.GetLocalIP(options.LocalIP)

	port := options.Port
	if options.Port == "" {
		port = "0" // go binds to an automatic port
	}

	// print dependencies, not used currently
	// pwd, _ := os.Getwd()
	// getDeps(pwd, options.Kitename)

	messenger := NewHTTPMessenger(kiteID)

	messenger.Subscribe(kiteID)
	messenger.Subscribe("all")

	k := &Kite{
		Username:       options.Username,
		Kitename:       options.Kitename,
		Version:        options.Version,
		Uuid:           kiteID,
		PublicKey:      publicKey,
		Addr:           localIP + ":" + port,
		PublicIP:       publicIP,
		LocalIP:        localIP,
		Port:           port,
		Hostname:       hostname,
		Server:         rpc.NewServer(),
		KontrolEnabled: true,
		Messenger:      messenger,
		Clients:        NewClients(),
	}

	return k
}

// AddMethods is used to add new structs with exposed methods with a different
// name. rcvr is a struct on which your exported method's are defined. methods
// is a map that expose your methods with different names to the outside world.
func (k *Kite) AddMethods(rcvr interface{}, methods map[string]string) error {
	if rcvr == nil {
		return errors.New("method struct should not be nil")
	}

	k.Methods = k.createMethodMap(rcvr, methods)
	return k.Server.RegisterName(k.Kitename, rcvr)
}

// Start is a blocking method. It runs the kite server and then accepts requests
// asynchronously. It can be started in a goroutine if you wish to use kite as a
// client too.
func (k *Kite) Start() {
	// Start our blocking subscriber loop. We except messages in the format of:
	// filter:msg, where msg is in format JSON  of PubResponse protocol format.
	// Latter is important to ensure robustness, if not we have to unmarshal or
	// check every incoming message.
	if !k.KontrolEnabled {
		k.Registered = true
		k.serve(k.Addr)
	} else {
		k.Messenger.Consume(k.handle)
	}
}

// handle is a method that interprets the incoming message from Kontrol. The
// incoming message is in form of protocol.PubResponse.
func (k *Kite) handle(msg []byte) {
	var r protocol.PubResponse
	err := json.Unmarshal(msg, &r)
	if err != nil {
		slog.Println(err)
		return
	}

	// treat any incoming data as a ping, don't just rely on ping command
	// this makes the kite more robust if we can't catch one of the pings.
	k.Pong()

	switch r.Action {
	case protocol.AddKite:
		k.AddKite(r)
	case protocol.RemoveKite:
		k.RemoveKite(r)
	case protocol.UpdateKite:
		k.Registered = false //trigger reinitialization
	case "ping":
		// This is needed for Node Coordination, that means we register ourself
		// only if we got an "hello" from one of the kontrol servers. This is
		// needed in order to catch all PUB messages from Kontrol. For more
		// information about this pattern read "Node Coordination" from the Zmq
		// Guide.
		k.InitializeKite()
	default:
		return
	}

}

// AddKite is executed when a protocol.AddKite message has been received
// trough the handler.
func (k *Kite) AddKite(r protocol.PubResponse) {
	if !k.Registered {
		return
	}

	kite := &models.Kite{
		Base: protocol.Base{
			Username: r.Username,
			Kitename: r.Kitename,
			Token:    r.Token,
			Version:  r.Version,
			Uuid:     r.Uuid,
			Hostname: r.Hostname,
			Addr:     r.Addr,
		},
	}

	kites.Add(kite)

	// Groupache settings, enable when ready
	// k.SetPeers(k.PeersAddr()...)

	slog.Printf("[%s] -> known peers -> %v\n", r.Action, k.PeersAddr())
}

// RemoveKite is executed when a protocol.AddKite message has been received
// trough the handler.
func (k *Kite) RemoveKite(r protocol.PubResponse) {
	if !k.Registered {
		return
	}

	kites.Remove(r.Uuid)
	slog.Printf("[%s] -> known peers -> %v\n", r.Action, k.PeersAddr())
}

// Pong sends a 'pong' message whenever the kite receives a message from Kontrol.
// This is used for node coordination and notifier Kontrol that the Kite is alive.
func (k *Kite) Pong() {
	m := protocol.Request{
		Base: protocol.Base{
			Kitename: k.Kitename,
			Uuid:     k.Uuid,
		},
		Action: "pong",
	}

	msg, _ := json.Marshal(&m)

	resp := k.Messenger.Send(msg)
	if string(resp) == "UPDATE" {
		k.Registered = false
	}
}

// InitializeKite runs the builtin RPC server and also registers itself to Kontrol
// when the kite.KontrolEnabled flag is enabled. This method is non-blocking.
func (k *Kite) InitializeKite() {
	if k.Registered {
		return
	}

	slog.Println("not registered, sending register request to kontrol...")
	err := k.RegisterToKontrol()
	if err != nil {
		slog.Println(err)
		return
	}

	onceBody := func() { k.serve(k.Addr) }
	go k.OnceServer.Do(onceBody)

	k.Registered = true
}

// RegisterToKontrol sends a register message to Kontrol. It returns an error
// when it is not allowed by Kontrol. If allowed, nil is returned.
func (k *Kite) RegisterToKontrol() error {
	// Wait until the servers are ready
	m := protocol.Request{
		Base: protocol.Base{
			Username:  k.Username,
			Kitename:  k.Kitename,
			Version:   k.Version,
			Uuid:      k.Uuid,
			PublicKey: k.PublicKey,
			Hostname:  k.Hostname,
			Addr:      k.Addr,
			LocalIP:   k.LocalIP,
			PublicIP:  k.PublicIP,
			Port:      k.Port,
		},
		Action: "register",
	}

	msg, err := json.Marshal(&m)
	if err != nil {
		slog.Println("kontrolRequest marshall err", err)
		return err
	}

	// what if it times out?
	result := k.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	err = json.Unmarshal(result, &resp)
	if err != nil {
		return err
	}

	switch resp.Result {
	case protocol.AllowKite:
		slog.Printf("registered to kontrol: \n  Addr\t\t: %s\n  Version\t: %s\n  Uuid\t\t: %s\n\n", k.Addr, k.Version, k.Uuid)
		k.Username = resp.Username // we know now which user that is
		return nil
	case protocol.PermitKite:
		return errors.New("no permission to run")
	}

	return errors.New("got a nonstandard response")
}

/******************************************

RPC

******************************************/

// Can connect to RPC service using HTTP CONNECT to rpcPath.
var connected = "200 Connected to Go RPC"

// serve starts our rpc server with the given addr. Addr should be in form of
// "ip:port"
func (k *Kite) serve(addr string) {
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		slog.Fatalln("PANIC!!!!! RPC SERVER COULD NOT INITIALIZED:", err)
		return
	}

	k.Addr = listener.Addr().String()
	slog.Println("serve addr is", k.Addr)

	// GroupCache settings, enable it when ready
	// k.newPool(k.Addr) // registers to http.DefaultServeMux
	// k.newGroup()

	http.Handle(rpc.DefaultRPCPath, k)
	http.Serve(listener, nil)
}

// ServeHTTP interface for http package.
func (k *Kite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == protocol.WEBSOCKET_PATH {
		websocket.Handler(k.serveWS).ServeHTTP(w, r)
		return
	}

	slog.Println("a new rpc call is done from", r.RemoteAddr)
	if r.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must CONNECT\n")
		return
	}

	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		slog.Println("rpc hijacking ", r.RemoteAddr, ": ", err.Error())
		return
	}

	io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	k.Server.ServeCodec(NewKiteServerCodec(k, conn))
}

// serveWS is used serving content over WebSocket. Is used internally via
// ServeHTTP method.
func (k *Kite) serveWS(ws *websocket.Conn) {
	addr := ws.Request().RemoteAddr
	slog.Printf("[%s] client connected\n", addr)

	k.Clients.Add(&client{Conn: ws, Addr: addr})

	// k.Server.ServeCodec(NewJsonServerCodec(k, ws))
	k.Server.ServeCodec(NewDnodeServerCodec(k, ws))
}

type Remote struct {
	Username string
	Kitename string
	Kites    []*models.Kite
}

// Remote is used to create a new remote struct that is used for remote
// kite-to-kite calls.
func (k *Kite) Remote(username, kitename string) (*Remote, error) {
	remoteKites := k.requestKites(username, kitename)
	if len(remoteKites) == 0 {
		return nil, fmt.Errorf("no remote kites available for %s/%s", username, kitename)
	}

	return &Remote{
		Username: username,
		Kitename: kitename,
		Kites:    remoteKites,
	}, nil
}

// CallSync makes a blocking request to another kite. args and result is used
// by the remote kite, therefore you should know what the kite is expecting.
func (r *Remote) CallSync(method string, args interface{}, result interface{}) error {
	remoteKite, err := r.getClient()
	if err != nil {
		return err
	}

	rpcMethod := r.Kitename + "." + method
	err = remoteKite.Client.Call(rpcMethod, args, result)
	if err != nil {
		return fmt.Errorf("can't call '%s', err: %s", r.Kitename, err.Error())
	}

	return nil
}

// Call makes a non-blocking request to another kite. args is used by the
// remote kite, therefore you should know what the kite is expecting.  fn is a
// callback that is executed when the result and error has been received.
// Currently only string as a result is supported, but it needs to be changed.
func (r *Remote) Call(method string, args interface{}, fn func(err error, res string)) (*rpc.Call, error) {
	remoteKite, err := r.getClient()
	if err != nil {
		return nil, err
	}

	var response string

	request := &protocol.KiteRequest{
		Base: protocol.Base{
			Token: remoteKite.Token,
			Uuid:  remoteKite.Uuid,
		},
		Args:   args,
		Origin: protocol.ORIGIN_GOB,
	}

	rpcMethod := r.Kitename + "." + method
	d := remoteKite.Client.Go(rpcMethod, request, &response, nil)

	select {
	case <-d.Done:
		fn(d.Error, response)
	case <-time.Tick(10 * time.Second):
		fn(d.Error, response)
	}

	return d, nil
}

func (k *Kite) requestKites(username, kitename string) []*models.Kite {
	remoteKites := getKitesBy(username, kitename)
	if len(remoteKites) != 0 {
		return remoteKites
	}

	m := protocol.Request{
		Base: protocol.Base{
			Username: username,
			Kitename: k.Kitename,
			Version:  k.Version,
			Uuid:     k.Uuid,
			Hostname: k.Hostname,
			Addr:     k.Addr,
		},
		RemoteKite: kitename,
		Action:     "getKites",
	}

	msg, err := json.Marshal(&m)
	if err != nil {
		slog.Println("requestKites marshall err 1", err)
		return nil
	}

	slog.Println("sending requesting message...")
	result := k.Messenger.Send(msg)

	var kitesResp []protocol.PubResponse
	err = json.Unmarshal(result, &kitesResp)
	if err != nil {
		slog.Println("requestKites marshall err 2", err)
		return nil
	}

	for _, r := range kitesResp {
		kite := &models.Kite{
			Base: protocol.Base{
				Username: r.Username,
				Kitename: r.Kitename,
				Token:    r.Token,
				Version:  r.Version,
				Uuid:     r.Uuid,
				Hostname: r.Hostname,
				Addr:     r.Addr,
			},
		}

		kites.Add(kite)
	}

	return getKitesBy(username, kitename)
}

func (r *Remote) getClient() (*models.Kite, error) {
	kite, err := r.roundRobin()
	if err != nil {
		return nil, err
	}

	if kite.Client == nil {
		var err error

		slog.Printf("establishing HTTP client conn for %s - %s on %s\n",
			kite.Kitename, kite.Addr, kite.Hostname)

		kite.Client, err = r.dialRemote(kite.Addr)
		if err != nil {
			return nil, err
		}

		// update kite in storage after we have an established connection
		kites.Add(kite)
	}

	return kite, nil
}

func (r *Remote) roundRobin() (*models.Kite, error) {
	if len(r.Kites) == 0 {
		return nil, fmt.Errorf("kite %s/%s does not exist", r.Username, r.Kitename)
	}

	// TODO: use container/ring :)
	index := balance.GetIndex(r.Kitename)
	N := float64(len(r.Kites))
	n := int(math.Mod(float64(index+1), N))
	balance.AddOrUpdateIndex(r.Kitename, n)
	return r.Kites[n], nil
}

// dialRemote is used to connect to a Remote Kite via the GOB codec. This is
// used by other external kite methods.
func (r *Remote) dialRemote(addr string) (*rpc.Client, error) {
	var err error
	conn, err := net.Dial("tcp4", addr)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+rpc.DefaultRPCPath+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		codec := NewKiteClientCodec(conn) // pass our custom codec
		return rpc.NewClientWithCodec(codec), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  "tcp " + addr,
		Addr: nil,
		Err:  err,
	}
}

/******************************************

GroupCache

******************************************/
func (k *Kite) newPool(addr string) {
	k.Pool = groupcache.NewHTTPPool(addr)
}

func (k *Kite) newGroup() {
	k.Group = groupcache.NewGroup(k.Kitename, 64<<20, groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			dest.SetString("fatih")
			return nil
		}))
}

func (k *Kite) GetString(name, key string) (result string) {
	if k.Group == nil {
		return
	}

	k.Group.Get(nil, key, groupcache.StringSink(&result))
	return
}

func (k *Kite) GetByte(name, key string) (result []byte) {
	if k.Group == nil {
		return
	}

	k.Group.Get(nil, key, groupcache.AllocatingByteSliceSink(&result))
	return
}

func (k *Kite) SetPeers(peers ...string) {
	k.Pool.Set(peers...)
}

func (k *Kite) PeersAddr() []string {
	list := make([]string, 0)
	for _, kite := range kites.List() {
		list = append(list, kite.Addr)
	}
	return list
}

/******************************************

Misc

******************************************/

func (k *Kite) createMethodMap(rcvr interface{}, methods map[string]string) map[string]string {
	kiteStruct := reflect.TypeOf(rcvr)

	methodsMapping := make(map[string]string)
	for alternativeName, method := range methods {
		m, ok := kiteStruct.MethodByName(method)
		if !ok {
			slog.Printf("warning: no method with name: %s\n", method)
			continue
		}

		// map alternativeName to go's net/rpc methodname
		methodsMapping[alternativeName] = k.Kitename + "." + m.Name
	}

	return methodsMapping
}

// return kites from the storage that matches username and kitename
func getKitesBy(username, kitename string) []*models.Kite {
	remoteKites := make([]*models.Kite, 0)

	for _, r := range kites.List() {
		if r.Username == username && r.Kitename == kitename {
			remoteKites = append(remoteKites, r)
		}
	}

	return remoteKites
}
