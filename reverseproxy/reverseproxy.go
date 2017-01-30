package reverseproxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
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
	kites   map[string]url.URL
	kitesMu sync.Mutex

	// muxer for proxy
	mux            *http.ServeMux
	websocketProxy http.Handler
	httpProxy      http.Handler

	// Proxy properties used to give urls and bind the listener
	Scheme     string
	PublicHost string // If given it must match the domain in certificate.
	PublicPort int    // Uses for registering and defining the public port.
}

func New(conf *config.Config) *Proxy {
	k := kite.New(Name, Version)
	k.Config = conf

	p := &Proxy{
		Kite:   k,
		kites:  make(map[string]url.URL),
		readyC: make(chan bool),
		closeC: make(chan bool),
		mux:    http.NewServeMux(),
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
		// we don't use https explicitly, ssl termination is done here
		req.URL.Scheme = "ws"
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

	p.kites[r.Client.ID] = *kiteUrl

	proxyURL := url.URL{
		Scheme: p.Scheme,
		Host:   p.PublicHost + ":" + strconv.Itoa(p.PublicPort),
		Path:   "/proxy/" + r.Client.ID,
	}

	s := proxyURL.String()
	p.Kite.Log.Info("Registering kite with url: '%s'. Can be reached now with: '%s'", kiteUrl, s)

	return s, nil
}

func (p *Proxy) backend(req *http.Request) *url.URL {
	withoutProxy := strings.TrimPrefix(req.URL.Path, "/proxy")
	paths := strings.Split(withoutProxy, "/")

	if len(paths) == 0 {
		p.Kite.Log.Error("Invalid path '%s'", req.URL.String())
		return nil
	}

	// remove the first empty path
	paths = paths[1:]

	// get our kiteId and individuals paths
	kiteId, rest := paths[0], path.Join(paths[1:]...)

	p.Kite.Log.Info("[%s] Incoming proxy request for scheme: '%s', endpoint '/%s'",
		kiteId, req.URL.Scheme, rest)

	p.kitesMu.Lock()
	defer p.kitesMu.Unlock()

	backendURL, ok := p.kites[kiteId]
	if !ok {
		p.Kite.Log.Error("kite for id '%s' is not found: %s", kiteId, req.URL.String())
		return nil
	}

	// backendURL.Path contains the baseURL, like "/kite" and rest contains
	// SockJS related endpoints, like /info or /123/kjasd213/websocket
	backendURL.Scheme = req.URL.Scheme
	backendURL.Path += "/" + rest

	p.Kite.Log.Info("[%s] Proxying to backend url: '%s'.", kiteId, backendURL.String())
	return &backendURL
}

func (p *Proxy) director(req *http.Request) {
	u := p.backend(req)
	if u == nil {
		return
	}

	// we don't use https explicitly, ssl termination is done here
	req.URL.Scheme = "http"
	req.URL.Host = u.Host
	req.URL.Path = u.Path
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
