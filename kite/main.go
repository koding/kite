package kite

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/groupcache"
	uuid "github.com/nu7hatch/gouuid"
	zmq "github.com/pebbe/zmq3"
	"io"
	"koding/db/models"
	"koding/newkite/balancer"
	"koding/newkite/peers"
	"koding/newkite/protocol"
	"log"
	"math"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	routerKontrol = "tcp://127.0.0.1:5556"
	subKontrol    = "tcp://127.0.0.1:5557"
	kites         = peers.New()
	balance       = balancer.New()
)

type Kite struct {
	Username     string // user that calls/runs the kite
	Kitename     string // kites of same name can share memory with each other
	Uuid         string // unique Uuid of Kite, is generated on Start for now
	Token        string // temporary token, can be changed anytime
	Addr         string // RPC and GroupCache addresses
	PublicKey    string
	Hostname     string
	LocalIP      string // local network interface
	PublicIP     string // public reachable IP
	Port         string // port, that the kite is going to be run
	Version      string
	Dependencies string // other kites that needs to be run, in order to run this one
	Registered   bool   // registered is true if the Kite is registered to kontrol itself

	Pool       *groupcache.HTTPPool
	Group      *groupcache.Group
	Server     *rpc.Server
	Subscriber *zmq.Socket
	Dealer     *zmq.Socket
	OnceServer sync.Once // used to start the server only once
	OnceCall   sync.Once // used when multiple goroutines are requesting information from kontrol
	sync.Mutex           // protects zmq send for DEALER socket
}

func New(o *protocol.Options, method interface{}) *Kite {

	var err error
	if o == nil {
		o, err = readOptions("manifest.json")
		if err != nil {
			log.Fatal("error: could not read config file", err)
		}
	}

	// some simple validations for config
	if o.Username == "" || o.Kitename == "" {
		log.Fatal("error: options data is not set properly")
	}

	hostname, _ := os.Hostname()
	id, _ := uuid.NewV4()
	kiteID := id.String()

	sub, _ := zmq.NewSocket(zmq.SUB)
	sub.Connect(subKontrol)

	// set three filters
	sub.SetSubscribe(kiteID)     // individual, just for me
	sub.SetSubscribe(o.Kitename) // same type, kites with the same name
	sub.SetSubscribe("all")      // for all kites

	dealer, _ := zmq.NewSocket(zmq.DEALER)
	dealer.SetIdentity(kiteID) // use our ID also for zmq envelope
	dealer.Connect(routerKontrol)

	publicKey, err := getKey("public")
	if err != nil {
		log.Fatal("public key reading:", err)
	}

	publicIP := getPublicIP(o.PublicIP)
	localIP := getLocalIP(o.LocalIP)

	port := o.Port
	if o.Port == "" {
		port = "0" // binds to an automatic port
	}

	// print dependencies
	pwd, _ := os.Getwd()
	getDeps(pwd, o.Kitename)

	k := &Kite{
		Username:   o.Username,
		Kitename:   o.Kitename,
		Version:    o.Version,
		Uuid:       kiteID,
		PublicKey:  publicKey,
		Addr:       localIP + ":" + port,
		PublicIP:   publicIP,
		LocalIP:    localIP,
		Port:       port,
		Hostname:   hostname,
		Server:     rpc.NewServer(),
		Subscriber: sub,
		Dealer:     dealer,
	}

	if method != nil {
		k.AddFunction(o.Kitename, method)
	}

	return k
}

func (k *Kite) Start() {
	// Start our blocking subscriber loop. We except messages in the format of:
	// filter:msg, where msg is in format JSON  of PubResponse protocol format.
	// Latter is important to ensure robustness, if not we have to unmarshal or
	// check every incoming message.
	for {
		msg, _ := k.Subscriber.RecvBytes(0)
		frames := bytes.SplitN(msg, []byte(protocol.FRAME_SEPARATOR), 2)
		if len(frames) != 2 { // msg is malformed
			continue
		}

		filter := frames[0] // either "all" or k.Uuid (just for this kite)
		switch string(filter) {
		case "all", k.Uuid, k.Kitename:
			msg = frames[1] // msg is in JSON format
			k.handle(msg)
		default:
			debug("not intended for me, dropping", string(filter))
		}

	}
}

func (k *Kite) handle(msg []byte) {
	var r protocol.PubResponse
	err := json.Unmarshal(msg, &r)
	if err != nil {
		log.Println(err)
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
		k.Registered = false //trigger reinilization
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

func (k *Kite) AddKite(r protocol.PubResponse) {
	if !k.Registered {
		return
	}

	kite := &models.Kite{
		Base: protocol.Base{
			Username: r.Username,
			Kitename: r.Kitename,
			Version:  r.Version,
			Uuid:     r.Uuid,
			Token:    r.Token,
			Hostname: r.Hostname,
			Addr:     r.Addr,
		},
	}

	kites.Add(kite)
	k.SetPeers(k.PeersAddr()...)

	debug("[%s] -> known peers -> %v\n", r.Action, k.PeersAddr())
}

func (k *Kite) RemoveKite(r protocol.PubResponse) {
	if !k.Registered {
		return
	}

	kites.Remove(r.Uuid)
	permissions.Remove(r.Kitename)
	debug("[%s] -> known peers -> %v\n", r.Action, k.PeersAddr())
}

func (k *Kite) Pong() {
	m := protocol.Request{
		Base: protocol.Base{
			Kitename: k.Kitename,
			Uuid:     k.Uuid,
		},
		Action: "pong",
	}

	msg, _ := json.Marshal(&m)

	resp := k.MakeRequest(msg)
	if string(resp) == "UPDATE" {
		k.Registered = false
	}
}

func (k *Kite) InitializeKite() {
	if !k.Registered {
		debug("not registered, sending register request to kontrol...")

		err := k.RegisterToKontrol()
		if err != nil {
			fmt.Println(err)
			return
		}

		onceBody := func() { k.Serve(k.Addr) }
		go k.OnceServer.Do(onceBody)

		k.Registered = true
	}
}

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
		log.Println("kontrolRequest marshall err", err)
		return err
	}

	result := k.MakeRequest(msg)
	var resp protocol.RegisterResponse
	err = json.Unmarshal(result, &resp)
	if err != nil {
		return err
	}

	switch resp.Result {
	case protocol.AllowKite:
		fmt.Printf("registered to kontrol: \n  Addr\t\t: %s\n  Version\t: %s\n  Uuid\t\t: %s\n\n", k.Addr, k.Version, k.Uuid)
		return nil
	case protocol.PermitKite:
		return errors.New("no permission to run")
	default:
		return errors.New("got a nonstandart response")
	}

	return nil
}

func (k *Kite) MakeRequest(msg []byte) []byte {
	k.Lock()

	k.Dealer.SendBytes([]byte(""), zmq.SNDMORE)
	k.Dealer.SendBytes(msg, 0)

	k.Dealer.RecvBytes(0) // envelope delimiter
	reply, _ := k.Dealer.RecvBytes(0)
	k.Unlock()
	return reply
}

/******************************************

RPC

******************************************/

// Can connect to RPC service using HTTP CONNECT to rpcPath.
var connected = "200 Connected to Go RPC"

func (k *Kite) DialClient(kite *models.Kite) (*rpc.Client, error) {
	debug("establishing HTTP client conn for %s - %s on %s\n", kite.Kitename, kite.Addr, kite.Hostname)
	var err error
	conn, err := net.Dial("tcp4", kite.Addr)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+rpc.DefaultRPCPath+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		c := NewKiteClientCodec(k, conn) // pass our custom codec
		return rpc.NewClientWithCodec(c), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  "tcp " + kite.Addr,
		Addr: nil,
		Err:  err,
	}
}

func (k *Kite) Serve(addr string) {
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		log.Println("PANIC!!!!! RPC SERVER COULD NOT INITIALIZED:", err)
		return
	}

	k.Addr = listener.Addr().String()
	fmt.Println("serve addr is", k.Addr)

	// GroupCache
	k.NewPool(k.Addr)
	k.NewGroup()

	k.Server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
	http.Serve(listener, k)
}

func (k *Kite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == protocol.WEBSOCKET_PATH {
		websocket.Handler(k.ServeWS).ServeHTTP(w, r)
		return
	}

	debug("a new rpc call is done from", r.RemoteAddr)
	if r.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must CONNECT\n")
		return
	}

	debug("hijacking conn")
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", r.RemoteAddr, ": ", err.Error())
		return
	}

	io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	k.Server.ServeCodec(NewKiteServerCodec(k, conn))

}

func (k *Kite) ServeWS(ws *websocket.Conn) {
	debug("client websocket connection from %v - %s \n", ws.RemoteAddr(), ws.Request().RemoteAddr)
	addr := ws.Request().RemoteAddr

	bufClients.add(&client{Conn: ws, Addr: addr})
	fmt.Println("number of buffered clients", bufClients.size())

	k.Server.ServeCodec(NewJsonServerCodec(k, ws))
}

func (k *Kite) AddFunction(name string, method interface{}) {
	k.Server.RegisterName(name, method)
}

func (k *Kite) CallSync(kite, method string, args interface{}, result interface{}) error {
	remoteKite, err := k.GetRemoteKite(kite)
	if err != nil {
		return err
	}

	rpcFunc := kite + "." + method
	err = remoteKite.Client.Call(rpcFunc, args, result)
	if err != nil {
		log.Println(err)
		return fmt.Errorf("[%s] call error: %s", kite, err.Error())
	}

	return nil
}

func (k *Kite) Call(kite, method string, args interface{}, fn func(err error, res string)) *rpc.Call {
	rpcFunc := kite + "." + method
	ticker := time.NewTicker(time.Second * 1)
	runCall := make(chan bool, 1)
	resetOnce := make(chan bool, 1)

	var remoteKite *models.Kite
	var err error

	for {
		select {
		case <-ticker.C:
			remoteKite, err = k.GetRemoteKite(kite)
			if err != nil {
				debug("no remote kites available, requesting some ...")
				m := protocol.Request{
					Base: protocol.Base{
						Username: k.Username,
						Kitename: k.Kitename,
						Version:  k.Version,
						Uuid:     k.Uuid,
						Hostname: k.Hostname,
						Addr:     k.Addr,
					},
					RemoteKite: kite,
					Action:     "getKites",
				}

				msg, err := json.Marshal(&m)
				if err != nil {
					log.Println("kontrolRequest marshall err", err)
					continue
				}

				onceBody := func() {
					debug("sending requesting message...")
					k.MakeRequest(msg)
				}

				k.OnceCall.Do(onceBody) // to prevent multiple get request when called conccurently
			} else {
				ticker.Stop()
				debug("making rpc call to '%s' with token '%s': -> ", remoteKite.Kitename, remoteKite.Token)
				runCall <- true
				resetOnce <- true
			}
		case <-runCall:
			var result string

			a := &protocol.KiteRequest{
				Base: protocol.Base{
					Username: k.Username,
					Kitename: k.Kitename,
					Version:  k.Version,
					Token:    remoteKite.Token,
					Uuid:     k.Uuid,
					Hostname: k.Hostname,
				},
				Args:   args,
				Origin: protocol.ORIGIN_GOB,
			}

			d := remoteKite.Client.Go(rpcFunc, a, &result, nil)

			select {
			case <-d.Done:
				fn(d.Error, result)
			case <-time.Tick(10 * time.Second):
				fn(d.Error, result)
			}
			return d
		case <-resetOnce:
			k.OnceCall = sync.Once{}
		}
	}
}

func (k *Kite) GetRemoteKite(kite string) (*models.Kite, error) {
	r, err := k.RoundRobin(kite)
	if err != nil {
		return nil, err
	}

	if r.Client == nil {
		var err error
		r.Client, err = k.DialClient(r)
		if err != nil {
			return nil, err
		}
		kites.Add(r)
	}

	return r, nil
}

func (k *Kite) RoundRobin(kite string) (*models.Kite, error) {
	// TODO: use cointainer/ring :)
	remoteKites := k.RemoteKites(kite)
	lenOfKites := len(remoteKites)
	if lenOfKites == 0 {
		return nil, fmt.Errorf("kite %s does not exist", kite)
	}

	index := balance.GetIndex(kite)
	N := float64(lenOfKites)
	n := int(math.Mod(float64(index+1), N))
	balance.AddOrUpdateIndex(kite, n)
	return remoteKites[n], nil
}

func (k *Kite) RemoteKites(kite string) []*models.Kite {
	l := kites.List()
	remoteKites := make([]*models.Kite, 0, len(l)-1) // allocate one less, it's the kite itself

	for _, r := range l {
		if r.Kitename == kite {
			remoteKites = append(remoteKites, r)
		}
	}

	return remoteKites
}

/******************************************

GroupCache

******************************************/
func (k *Kite) NewPool(addr string) {
	k.Pool = groupcache.NewHTTPPool(addr)
}

func (k *Kite) NewGroup() {
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

WebSocket Clients

******************************************/

type wsClients map[string][]*client
type tempClients map[string]*client

type client struct {
	Addr     string
	Username string
	Conn     *websocket.Conn
}

var clientsMu sync.Mutex // protectx wsClients, tempClients

var clients = make(wsClients)      // registered clients
var bufClients = make(tempClients) // unregistered clients

// Temporary Buffered clients
func (t tempClients) size() int {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	return len(t)
}

func (t tempClients) add(c *client) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	t[c.Addr] = c
}

func (t tempClients) remove(addr string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	delete(t, addr)
}

func (t tempClients) get(addr string) *client {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	c, ok := t[addr]
	if !ok {
		return nil
	}

	return c
}

// Registered clients

func (w wsClients) add(lookup string, c *client) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	lookup = strings.ToLower(lookup)

	_, ok := w[lookup]
	if !ok {
		w[lookup] = make([]*client, 0)
		w[lookup] = append(w[lookup], c)
	} else {
		w[lookup] = append(w[lookup], c)
	}
}

func (w wsClients) get(addr string) []*client {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	c, ok := w[addr]
	if !ok {
		return nil
	}

	return c
}

func (w wsClients) remove(username string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	_, ok := w[username]
	if !ok {
		return
	}

	delete(w, username)
}

func (w wsClients) has(username string) bool {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	_, ok := w[username]
	return ok
}

func (w wsClients) size() int {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	return len(w)
}

func (w wsClients) broadcast(msg string) {
	clientsMu.Lock()
	for _, cs := range w {
		go func(cs []*client) {
			for _, c := range cs {
				if err := websocket.Message.Send(c.Conn, msg); err != nil {
					log.Println("Could not send message to ", c.Addr, err.Error())
				}

			}
		}(cs)
	}
	clientsMu.Unlock()
}

func (k *Kite) IsLogged(user string) bool {
	return clients.has(user)
}

func (k *Kite) Broadcast(msg string) {
	clients.broadcast(msg)
}

func (k *Kite) SendMsg(msg, user string) {
	for _, c := range clients.get(user) {
		if err := websocket.Message.Send(c.Conn, msg); err != nil {
			log.Println("Could not send message to ", c.Addr, err.Error())
		}
	}
}

func (k *Kite) ConnectedUsers() []string {
	list := make([]string, 0)
	clientsMu.Lock()
	for user := range clients {
		if user == "" {
			continue
		}

		list = append(list, user)
	}
	clientsMu.Unlock()

	return list
}
