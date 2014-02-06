// Package proxy implements a reverse-proxy for kites behind firewall or NAT.
package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"kite"
	"kite/protocol"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unsafe"

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
	urls map[*kite.RemoteKite]protocol.KiteURL
}

func New(tlsPort int, domain, key, cert string) *Proxy {
	options := &kite.Options{
		Kitename:    "proxy",
		Version:     "0.0.1",
		Environment: "production",
		Region:      "localhost",
		Visibility:  protocol.Public,
	}

	proxyKite := &Proxy{
		kite:    kite.New(options),
		tlsPort: tlsPort,
		domain:  domain,
		key:     key,
		cert:    cert,
		urls:    make(map[*kite.RemoteKite]protocol.KiteURL),
	}

	proxyKite.kite.HandleFunc("register", proxyKite.register)

	// Remove URL from the map when Kite disconnects.
	proxyKite.kite.OnDisconnect(func(r *kite.RemoteKite) { delete(proxyKite.urls, r) })

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

	cert, err := tls.LoadX509KeyPair(config.Current.Proxy.CertFile, config.Current.Proxy.KeyFile)
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

func (t *Proxy) register(r *kite.Request) (interface{}, error) {
	t.urls[r.RemoteKite] = r.RemoteKite.URL

	path := strings.Join([]string{
		"proxy",
		fmt.Sprintf("%d", unsafe.Pointer(r.RemoteKite)),
		r.RemoteKite.Kite.Username,
		r.RemoteKite.Kite.Environment,
		r.RemoteKite.Kite.Name,
		r.RemoteKite.Kite.Version,
		r.RemoteKite.Kite.Region,
		r.RemoteKite.Kite.Hostname,
		r.RemoteKite.Kite.ID,
	}, "/")

	result := url.URL{
		Scheme: "wss",
		Host:   net.JoinHostPort(config.Current.Proxy.Domain, strconv.Itoa(t.tlsPort)),
		Path:   path,
	}

	return result.String(), nil
}

func (t *Proxy) handleWS(ws *websocket.Conn) {
	path := ws.Request().URL.Path[1:] // strip leading '/'

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return
	}

	if parts[0] != "proxy" {
		return
	}

	i, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return
	}

	r := (*kite.RemoteKite)(unsafe.Pointer(uintptr(i)))
	kiteURL, ok := t.urls[r]
	if !ok {
		return
	}

	conn, err := websocket.Dial(kiteURL.String(), "kite", "http://localhost")
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
