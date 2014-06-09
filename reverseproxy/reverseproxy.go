package reverseproxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	mux *http.ServeMux

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
	proxy := &websocketproxy.WebsocketProxy{
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

	p.mux.Handle("/kite", k)
	p.mux.Handle("/proxy", proxy)

	// OnDisconnect is called whenever a kite is disconnected from us.
	k.OnDisconnect(func(r *kite.Client) {
		k.Log.Info("Removing kite Id '%s' from proxy. It's disconnected", r.Kite.ID)
		delete(p.kites, r.Kite.ID)
	})

	return p
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

func (p *Proxy) backend(req *http.Request) *url.URL {
	kiteId := req.URL.Query().Get("kiteId")

	p.kitesMu.Lock()
	defer p.kitesMu.Unlock()

	backendURL, ok := p.kites[kiteId]
	if !ok {
		p.Kite.Log.Error("kite for id '%s' is not found: %s", kiteId, req.URL.String())
		return nil
	}

	p.Kite.Log.Info("Returning backend url: '%s' for kiteId: %s", backendURL.String(), kiteId)

	return backendURL
}

// ListenAndServe listens on the TCP network address addr and then calls Serve
// with handler to handle requests on incoming connections.
func (p *Proxy) ListenAndServe() error {
	var err error
	p.listener, err = net.Listen("tcp",
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
