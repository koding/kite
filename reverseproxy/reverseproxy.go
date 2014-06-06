package reverseproxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/websocketproxy"
)

const (
	Version           = "0.0.1"
	Name              = "reverseproxy"
	DefaultPort       = 3999
	DefaultPublicHost = "localhost:3999"
)

type Proxy struct {
	Kite *kite.Kite

	listener  net.Listener
	TLSConfig *tls.Config

	// Holds registered kites. Keys are kite IDs.
	kites   map[string]*url.URL
	kitesMu sync.Mutex

	// muxer for proxy
	mux *http.ServeMux

	// If given it must match the domain in certificate.
	PublicHost string

	RegisterToKontrol bool

	// Proxy URL that get registered to Kontrol
	Url *url.URL
}

func New(conf *config.Config) *Proxy {
	k := kite.New(Name, Version)
	k.Config = conf

	// Listen on 3999 by default
	if k.Config.Port == 0 {
		k.Config.Port = DefaultPort
	}

	p := &Proxy{
		Kite:              k,
		kites:             make(map[string]*url.URL),
		mux:               http.NewServeMux(),
		RegisterToKontrol: true,
		PublicHost:        DefaultPublicHost,
	}

	// third part kites are going to use this to register themself to
	// proxy-kite and get a proxy url, which they use for register to kontrol.
	p.Kite.HandleFunc("register", p.handleRegister)

	// create our websocketproxy http.handler
	proxy := &websocketproxy.WebsocketProxy{
		Backend: p.backend,
	}

	p.mux.Handle("/kite", k)
	p.mux.Handle("/proxy", proxy)

	return p
}

func (p *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	p.kites[r.Client.ID] = r.Client.WSConfig.Location

	proxyURL := url.URL{
		Scheme:   p.Url.Scheme,
		Host:     p.Url.Host,
		Path:     "proxy",
		RawQuery: "kiteId=" + r.Client.ID,
	}

	return proxyURL.String(), nil
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

	return backendURL
}

func (p *Proxy) registerURL(scheme string) *url.URL {
	registerURL := p.Url
	if p.Url == nil {
		registerURL = &url.URL{
			Scheme: scheme,
			Host:   p.PublicHost,
			Path:   "/kite",
		}
	}

	return registerURL
}

// ListenAndServe listens on the TCP network address addr and then calls Serve
// with handler to handle requests on incoming connections. Handler is
// typically nil, in which case the DefaultServeMux is used.
func (p *Proxy) ListenAndServe() error {
	var err error
	p.listener, err = net.Listen("tcp",
		net.JoinHostPort(p.Kite.Config.IP, strconv.Itoa(p.Kite.Config.Port)))

	if err != nil {
		return err
	}

	if p.RegisterToKontrol {
		go p.Kite.RegisterForever(p.registerURL("ws"))
	}

	server := http.Server{
		Handler: p.mux,
	}

	defer p.Kite.Close()
	return server.Serve(p.listener)
}

func (p *Proxy) ListenAndServeTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		p.Kite.Log.Fatal(err.Error())
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	p.listener, err = net.Listen("tcp",
		net.JoinHostPort(p.Kite.Config.IP, strconv.Itoa(p.Kite.Config.Port)))
	if err != nil {
		p.Kite.Log.Fatal(err.Error())
	}

	p.listener = tls.NewListener(p.listener, tlsConfig)

	if p.RegisterToKontrol {
		go p.Kite.RegisterForever(p.registerURL("wss"))
	}

	server := &http.Server{
		Handler:   p.mux,
		TLSConfig: tlsConfig,
	}

	defer p.Kite.Close()
	return server.Serve(p.listener)
}
