// Package proxy implements a reverse-proxy for kites behind firewall or NAT.
package proxy

import (
	"crypto/tls"
	"io"
	"kite"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"code.google.com/p/go.net/websocket"
)

type Proxy struct {
	kite *kite.Kite

	tlsPort     int
	tlsListener net.Listener

	domain string
	key    string
	cert   string

	// Holds registered kites.
	urls map[string]*kite.RemoteKite
}

func New(kiteOptions *kite.Options, domain string, tlsPort int, certPEM, keyPEM string) *Proxy {
	proxyKite := &Proxy{
		kite:    kite.New(kiteOptions),
		tlsPort: tlsPort,
		domain:  domain,
		cert:    certPEM,
		key:     keyPEM,
		urls:    make(map[string]*kite.RemoteKite),
	}

	proxyKite.kite.HandleFunc("register", proxyKite.handleRegister)

	// Remove URL from the map when Kite disconnects.
	proxyKite.kite.OnDisconnect(func(r *kite.RemoteKite) { delete(proxyKite.urls, r.Kite.ID) })

	return proxyKite
}

func (t *Proxy) Run() {
	t.startHTTPSServer()
	t.kite.Run()
}

func (t *Proxy) Start() {
	t.startHTTPSServer()
	t.kite.Start()
}

func (t *Proxy) startHTTPSServer() {
	srv := &websocket.Server{Handler: t.handleWS}
	srv.Config.TlsConfig = &tls.Config{}

	cert, err := tls.X509KeyPair([]byte(t.cert), []byte(t.key))
	if err != nil {
		t.kite.Log.Fatal(err.Error())
	}

	srv.Config.TlsConfig.Certificates = []tls.Certificate{cert}

	addr := ":" + strconv.Itoa(t.tlsPort)
	t.tlsListener, err = net.Listen("tcp", addr)
	if err != nil {
		t.kite.Log.Fatal(err.Error())
	}

	t.tlsListener = tls.NewListener(t.tlsListener, srv.Config.TlsConfig)

	go func() {
		if err := http.Serve(t.tlsListener, srv); err != nil {
			t.kite.Log.Fatal(err.Error())
		}
	}()
}

func (t *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	t.urls[r.RemoteKite.Kite.ID] = r.RemoteKite

	proxyURL := url.URL{
		Scheme: "wss",
		Host:   net.JoinHostPort(t.domain, strconv.Itoa(t.tlsPort)),
		Path:   "proxy" + r.RemoteKite.Kite.Key(),
	}

	return proxyURL.String(), nil
}

func (t *Proxy) handleWS(ws *websocket.Conn) {
	path := ws.Request().URL.Path[1:] // strip leading "/"

	parts := strings.Split(path, "/")

	if len(parts) != 8 {
		return
	}

	if parts[0] != "proxy" { // path must start with "proxy"
		return
	}

	kiteID := parts[7]

	remoteKite, ok := t.urls[kiteID]
	if !ok {
		return
	}

	conn, err := websocket.Dial(remoteKite.Kite.URL.String(), "kite", "http://localhost")
	if err != nil {
		return
	}

	errc := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errc <- err
	}
	go cp(conn, ws)
	go cp(ws, conn)
	<-errc
}
