package reverseproxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/websocketproxy"
)

const (
	Version = "0.0.1"
	Name    = "proxy"
)

type Proxy struct {
	Kite *kite.Kite

	listener  net.Listener
	TLSConfig *tls.Config

	readyC chan bool // To signal when kite is ready to accept connections
	closeC chan bool // To signal when kite is closed with Close()

	// Holds registered kites. Keys are kite IDs.
	kites   map[string]*url.URL
	kitesMu sync.Mutex

	// muxer for proxy
	mux            *http.ServeMux
	websocketProxy http.Handler
	httpProxy      http.Handler

	// Proxy properties used to give urls and bind the listener
	Scheme     string
	PublicHost string // If given it must match the domain in certificate.
	Port       int
}

func New(conf *config.Config) *Proxy {
	k := kite.New(Name, Version)
	k.Config = conf

	// Listen on 3999 by default

	p := &Proxy{
		Kite:   k,
		kites:  make(map[string]*url.URL),
		readyC: make(chan bool),
		closeC: make(chan bool),
		mux:    http.NewServeMux(),
		Port:   k.Config.Port,
	}

	// third part kites are going to use this to register themself to
	// proxy-kite and get a proxy url, which they use for register to kontrol.
	p.Kite.HandleFunc("register", p.handleRegister)

	// create our websocketproxy http.handler

	p.websocketProxy = &websocketproxy.WebsocketProxy{
		Backend: p.backend,
		Upgrader: &websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: change this to publicdomain and also kites should add them to
				return true
			},
		},
	}

	p.httpProxy = &httputil.ReverseProxy{
		Director: p.director,
	}

	p.mux.Handle("/", k)
	p.mux.Handle("/proxy/", p)

	// OnDisconnect is called whenever a kite is disconnected from us.
	k.OnDisconnect(func(r *kite.Client) {
		k.Log.Info("Removing kite Id '%s' from proxy. It's disconnected", r.Kite.ID)
		delete(p.kites, r.Kite.ID)
	})

	return p
}

// ServeHTTP implements the http.Handler interface.
func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if isWebsocket(req) {
		p.websocketProxy.ServeHTTP(rw, req)
		return
	}

	p.httpProxy.ServeHTTP(rw, req)
}

// isWebsocket checks wether the incoming request is a part of websocket
// handshake
func isWebsocket(req *http.Request) bool {
	if strings.ToLower(req.Header.Get("Upgrade")) != "websocket" ||
		!strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") {
		return false
	}
	return true
}

func (p *Proxy) CloseNotify() chan bool {
	return p.closeC
}

func (p *Proxy) ReadyNotify() chan bool {
	return p.readyC
}

func (p *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	kiteUrl, err := url.Parse(r.Args.One().MustString())
	if err != nil {
		return nil, err
	}

	p.kites[r.Client.ID] = kiteUrl

	proxyURL := url.URL{
		Scheme:   p.Scheme,
		Host:     p.PublicHost + ":" + strconv.Itoa(p.Port),
		Path:     "proxy",
		RawQuery: "kiteId=" + r.Client.ID,
	}

	s := proxyURL.String()
	p.Kite.Log.Info("Registering kite with url: '%s'. Can be reached now with: '%s'", kiteUrl, s)

	return s, nil
}

func (p *Proxy) director(req *http.Request) {
	u := p.backend(req)
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.URL.Path = u.Path
}

func (p *Proxy) backend(req *http.Request) *url.URL {
	kiteId := req.URL.Query().Get("kiteId")

	p.kitesMu.Lock()
	defer p.kitesMu.Unlock()

	backendURL, ok := p.kites[kiteId]
	if !ok {
		p.Kite.Log.Error("kite for id '%s' is not found: %s", kiteId, req.URL.String())
		return nil
	}

	// change "http" with "ws" because websocket procol expects a ws or wss as
	// scheme
	if err := replaceSchemeWithWS(backendURL); err != nil {
		return nil
	}

	// change now the path for the backend kite. Kite register itself with
	// something like "localhost:7777/kite" however we are going to
	// dial/connect to a sockjs server and there is no sessionId/serverId in
	// the path. This causes problem because the SockJS serve can't parse it.
	// Therefore we as an intermediate client are getting the path as (ommited the query):
	// "/proxy/795/kite-fba0954a-07c7-4d34-4215-6a88733cf65c-OjLnvABL/websocket"
	// which will be converted to
	// "localhost:7777/kite/795/kite-fba0954a-07c7-4d34-4215-6a88733cf65c-OjLnvABL/websocket"
	backendURL.Path += strings.TrimPrefix(req.URL.Path, "/proxy")

	// also change the Origin to the client's host name, like as if someone
	// with the same backendUrl is trying to connect to the kite. Otherwise
	// will get an "Origin not allowed"
	req.Header.Set("Origin", "http://"+backendURL.Host)

	p.Kite.Log.Info("Returning backend url: '%s' for kiteId: %s", backendURL.String(), kiteId)
	return backendURL
}

// TODO: put this into a util package, is used by others too
func replaceSchemeWithWS(u *url.URL) error {
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("invalid scheme in url: %s", u.Scheme)
	}
	return nil
}

// ListenAndServe listens on the TCP network address addr and then calls Serve
// with handler to handle requests on incoming connections.
func (p *Proxy) ListenAndServe() error {
	var err error
	p.listener, err = net.Listen("tcp4",
		net.JoinHostPort(p.Kite.Config.IP, strconv.Itoa(p.Kite.Config.Port)))
	if err != nil {
		return err
	}
	p.Kite.Log.Info("Listening on: %s", p.listener.Addr().String())

	close(p.readyC)

	server := http.Server{
		Handler: p.mux,
	}

	defer close(p.closeC)
	return server.Serve(p.listener)
}

func (p *Proxy) ListenAndServeTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		p.Kite.Log.Fatal("Could not load cert/key files: %s", err.Error())
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	p.listener, err = net.Listen("tcp",
		net.JoinHostPort(p.Kite.Config.IP, strconv.Itoa(p.Kite.Config.Port)))
	if err != nil {
		p.Kite.Log.Fatal(err.Error())
	}
	p.Kite.Log.Info("Listening on: %s", p.listener.Addr().String())

	// now we are ready
	close(p.readyC)

	p.listener = tls.NewListener(p.listener, tlsConfig)

	server := &http.Server{
		Handler:   p.mux,
		TLSConfig: tlsConfig,
	}

	defer close(p.closeC)
	return server.Serve(p.listener)
}

func (p *Proxy) Run() {
	p.ListenAndServe()
}
