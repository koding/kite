package kite

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/golang/groupcache"
	"io"
	"koding/messaging/moh"
	"koding/newkite/peers"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"koding/tools/dnode"
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

	// registers to kontrol in this interval
	registerInterval = 700 * time.Millisecond

	// after hitting the limit the register interval is no more increased
	maxRegisterLimit = 30
)

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
	protocol.Kite

	// KodingKey is used for authenticate to Kontrol.
	KodingKey string

	// Local network interface address.
	// It will be populated after registering with Kontrol.
	LocalIP string

	// Registered is true if the Kite is registered to kontrol itself
	Registered bool

	// other kites that needs to be run, in order to run this one
	Dependencies string

	// kind is temporary field that is used for Koding client side functionality
	Kind string

	// by default yes, if disabled it bypasses kontrol
	KontrolEnabled bool

	// method map for shared methods
	Methods map[string]string

	// implements the Clients interface
	Clients Clients

	// GroupCache variables
	Pool  *groupcache.HTTPPool
	Group *groupcache.Group

	// RpcServer
	Server *rpc.Server

	// To allow only one register request at the same time
	registerMutex sync.Mutex

	// Used to talk with Kontrol server
	kontrolClient *moh.MessagingClient
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

	hostname, _ := os.Hostname()
	kiteID := utils.GenerateUUID()

	kodingKey, err := utils.GetKodingKey()
	if err != nil {
		slog.Fatalln("Couldn't find koding.key. Please run 'kd register'.")
	}

	port := options.Port
	if options.Port == "" {
		port = "0" // OS binds to an automatic port
	}

	var publicIP string
	if options.PublicIP == "" {
		publicIP = utils.GetLocalIP(options.LocalIP)
	} else {
		publicIP = options.PublicIP
	}

	if options.KontrolAddr == "" {
		options.KontrolAddr = "127.0.0.1:4000" // local fallback address
	}

	k := &Kite{
		Kite: protocol.Kite{
			Name:     options.Kitename,
			Username: options.Username,
			ID:       kiteID,
			Version:  options.Version,
			Hostname: hostname,
			PublicIP: publicIP,
			Port:     port,
		},
		Kind:           options.Kind,
		KodingKey:      kodingKey,
		Server:         rpc.NewServer(),
		KontrolEnabled: true,
		Methods:        make(map[string]string),
		Clients:        NewClients(),
	}

	k.kontrolClient = moh.NewMessagingClient(options.KontrolAddr, k.handle)
	k.kontrolClient.Subscribe(kiteID)
	k.kontrolClient.Subscribe("all")

	// Register our internal method
	k.Methods["vm.info"] = "status.Info"
	k.Server.RegisterName("status", new(Status))

	return k
}

// AddMethods is used to add new structs with exposed methods with a different
// name. rcvr is a struct on which your exported method's are defined. methods
// is a map that expose your methods with different names to the outside world.
func (k *Kite) AddMethods(rcvr interface{}, methods map[string]string) error {
	if rcvr == nil {
		panic(errors.New("method struct should not be nil"))
	}

	k.createMethodMap(rcvr, methods)
	return k.Server.RegisterName(k.Name, rcvr)
}

func (k *Kite) createMethodMap(rcvr interface{}, methods map[string]string) {
	kiteStruct := reflect.TypeOf(rcvr)

	for alternativeName, method := range methods {
		m, ok := kiteStruct.MethodByName(method)
		if !ok {
			panic(fmt.Sprintf("addmethods err: no method with name: %s\n", method))
			continue
		}

		// map alternativeName to go's net/rpc methodname
		k.Methods[alternativeName] = k.Name + "." + m.Name
	}
}

// Start is a blocking method. It runs the kite server and then accepts requests
// asynchronously. It can be started in a goroutine if you wish to use kite as a
// client too.
func (k *Kite) Start() {
	k.parseVersionFlag()

	// This is blocking
	k.serve()
}

// If the user wants to call flag.Parse() the flag must be defined in advance.
var _ = flag.Bool("version", false, "show version")

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

// handle is a method that interprets the incoming message from Kontrol. The
// incoming message must be in form of protocol.KontrolMessage.
func (k *Kite) handle(msg []byte) {
	var r protocol.KontrolMessage
	err := json.Unmarshal(msg, &r)
	if err != nil {
		slog.Println(err)
		return
	}
	// fmt.Printf("INCOMING KONTROL MSG: %+v\n", r)

	switch r.Type {
	case protocol.KiteRegistered:
		k.AddKite(r)
	case protocol.KiteDisconnected:
		k.RemoveKite(r)
	case protocol.KiteUpdated:
		k.Registered = false //trigger reinitialization
	case protocol.Ping:
		k.Pong()
	default:
		return
	}

}

func unmarshalKiteArg(r *protocol.KontrolMessage) *protocol.Kite {
	k := r.Args["kite"].(map[string]interface{})
	// Must set all fields manually
	return &protocol.Kite{
		Name:     k["name"].(string),
		Username: k["username"].(string),
		ID:       k["id"].(string),
		Kind:     k["kind"].(string),
		Version:  k["version"].(string),
		Hostname: k["hostname"].(string),
		PublicIP: k["publicIP"].(string),
		Port:     k["port"].(string),
	}
}

// AddKite is executed when a protocol.AddKite message has been received
// trough the handler.
func (k *Kite) AddKite(r protocol.KontrolMessage) {
	kite := unmarshalKiteArg(&r)

	kites.Add(kite)

	// Groupache settings, enable when ready
	// k.SetPeers(k.PeersAddr()...)

	slog.Printf("[%s] -> known peers -> %v\n", r.Type, k.PeersAddr())
}

// RemoveKite is executed when a protocol.AddKite message has been received
// trough the handler.
func (k *Kite) RemoveKite(r protocol.KontrolMessage) {
	kite := unmarshalKiteArg(&r)

	kites.Remove(kite.ID)
	slog.Printf("[%s] -> known peers -> %v\n", r.Type, k.PeersAddr())
}

// Pong sends a 'pong' message whenever the kite receives a message from Kontrol.
// This is used for node coordination and notifier Kontrol that the Kite is alive.
func (k *Kite) Pong() {
	m := protocol.KiteToKontrolRequest{
		Kite:   k.Kite,
		Method: protocol.Pong,
	}

	msg, _ := json.Marshal(&m)

	resp, _ := k.kontrolClient.Request(msg)
	if string(resp) == "UPDATE" {
		k.Registered = false
		k.registerToKontrol()
	}
}

// registerToKontrol sends a register message to Kontrol. It returns an error
// when it is not allowed by Kontrol. If allowed, nil is returned.
func (k *Kite) registerToKontrol() error {
	k.registerMutex.Lock()
	defer k.registerMutex.Unlock()

	if k.Registered {
		return nil
	}

	// Wait until the servers are ready
	m := protocol.KiteToKontrolRequest{
		Method:    protocol.RegisterKite,
		Kite:      k.Kite,
		KodingKey: k.KodingKey,
	}

	msg, err := json.Marshal(&m)
	if err != nil {
		slog.Println("kontrolRequest marshall err", err)
		return err
	}

	result, err := k.kontrolClient.Request(msg)
	if err != nil {
		return err
	}

	var resp protocol.RegisterResponse
	err = json.Unmarshal(result, &resp)
	if err != nil {
		return err
	}

	switch resp.Result {
	case protocol.AllowKite:
		slog.Printf("registered to kontrol: \n  Addr\t\t: %s\n  Version\t: %s\n  Uuid\t\t: %s\n\n", k.Addr(), k.Version, k.ID)
		k.Username = resp.Username // we know now which user that is
		k.Registered = true
		return nil
	case protocol.RejectKite:
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
func (k *Kite) serve() {
	listener, err := net.Listen("tcp4", k.Addr())
	if err != nil {
		slog.Fatalln("PANIC!!!!! RPC SERVER COULD NOT BE INITIALIZED:", err)
		return
	}

	slog.Println("serve addr is", k.Addr())

	// Port is known here if "0" is used as port number
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		slog.Fatalln("Invalid address")
	}

	k.PublicIP = host
	k.Port = port

	// We must connect to Kontrol after starting to listen on port
	if k.KontrolEnabled {
		// Listen Kontrol messages
		k.kontrolClient.Connect()
	}

	// GroupCache settings, enable it when ready
	// k.newPool(k.Addr) // registers to http.DefaultServeMux
	// k.newGroup()

	k.Server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)

	err = http.Serve(listener, k)
	if err != nil {
		slog.Fatalln(err)
	}
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

// broadcast sends messages in dnode protocol to all connected websocket
// clients method and arguments is mapped to dnode's method and arguments
// fields.
func (k *Kite) broadcast(method string, arguments interface{}) {
	for _, client := range k.Clients.List() {
		rawArgs, err := json.Marshal(arguments)
		if err != nil {
			fmt.Printf("collect json unmarshal %+v\n", err)
		}

		message := dnode.Message{
			Method:    "info",
			Arguments: &dnode.Partial{Raw: rawArgs},
			Links:     []string{},
			Callbacks: make(map[string][]string),
		}

		websocket.JSON.Send(client.Conn, message)
	}
}

// Remote encapsulates kites of specified type of specified user.
type Remote struct {
	Username string
	Kitename string
	Kites    []*RemoteKite
}

// RemoteKite is the structure representing other connected kites.
type RemoteKite struct {
	protocol.KiteWithToken
	Client *rpc.Client
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
		Kite:  remoteKite.Kite,
		Args:  args,
		Token: remoteKite.Token,
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

func (k *Kite) requestKites(username, kitename string) []*RemoteKite {
	m := protocol.KiteToKontrolRequest{
		Kite:      k.Kite,
		KodingKey: k.KodingKey,
		Method:    protocol.GetKites,
		Args: map[string]interface{}{
			"username": username,
			"kitename": kitename,
		},
	}

	msg, err := json.Marshal(&m)
	if err != nil {
		slog.Println("requestKites marshall err 1", err)
		return nil
	}

	slog.Println("sending requesting message...")
	result, err := k.kontrolClient.Request(msg)
	if err != nil {
		return nil
	}

	var kitesResp protocol.GetKitesResponse
	err = json.Unmarshal(result, &kitesResp)
	if err != nil {
		slog.Println("requestKites marshall err 2", err)
		return nil
	}

	remoteKites := make([]*RemoteKite, len(kitesResp))
	for i, k := range kitesResp {
		rk := &RemoteKite{
			KiteWithToken: k,
		}
		remoteKites[i] = rk
	}

	return remoteKites
}

func (r *Remote) getClient() (*RemoteKite, error) {
	kite, err := r.roundRobin()
	if err != nil {
		return nil, err
	}

	if kite.Client == nil {
		var err error

		slog.Printf("establishing HTTP client conn for %s - %s on %s\n",
			kite.Name, kite.Addr(), kite.Hostname)

		kite.Client, err = r.dialRemote(kite.Addr())
		if err != nil {
			return nil, err
		}

		// update kite in storage after we have an established connection
		kites.Add(&kite.Kite)
	}

	return kite, nil
}

func (r *Remote) roundRobin() (*RemoteKite, error) {
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
	k.Group = groupcache.NewGroup(k.Name, 64<<20, groupcache.GetterFunc(
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
		list = append(list, kite.Addr())
	}
	return list
}
