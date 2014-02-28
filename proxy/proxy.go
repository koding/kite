// Package proxy implements a reverse-proxy for kites behind firewall or NAT.
package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/registration"
)

const (
	Version     = "0.0.2"
	DefaultPort = 3999
)

type Proxy struct {
	Kite *kite.Kite

	listener  net.Listener
	TLSConfig *tls.Config

	IP   string
	Port int

	// TLS certificate
	key  string
	cert string

	// Must match the name in certificate.
	Domain string

	// For generating token tokens for tunnels.
	pubKey  string
	privKey string

	// Holds registered kites. Keys are kite IDs.
	kites map[string]*PrivateKite

	mux *http.ServeMux

	RegisterToKontrol bool

	url *url.URL
}

func New(conf *config.Config, pubKey, privKey string) *Proxy {
	k := kite.New("proxy", Version)
	k.Config = conf

	p := &Proxy{
		Kite:              k,
		pubKey:            pubKey,
		privKey:           privKey,
		kites:             make(map[string]*PrivateKite),
		IP:                "0.0.0.0",
		Port:              DefaultPort,
		mux:               http.NewServeMux(),
		RegisterToKontrol: true,
	}

	p.Kite.HandleFunc("register", p.handleRegister)

	p.mux.Handle("/kite", p.Kite)
	p.mux.Handle("/proxy", websocket.Server{Handler: p.handleProxy})   // Handler for clients outside
	p.mux.Handle("/tunnel", websocket.Server{Handler: p.handleTunnel}) // Handler for kites behind

	// Remove URL from the map when PrivateKite disconnects.
	k.OnDisconnect(func(r *kite.RemoteKite) {
		delete(p.kites, r.Kite.ID)
	})

	return p
}

func (p *Proxy) Close() {
	p.listener.Close()
	for _, k := range p.kites {
		k.Close()
		for _, t := range k.tunnels {
			t.Close()
		}
	}
}

func (p *Proxy) ListenAndServe() error {
	var err error
	p.listener, err = net.Listen("tcp", net.JoinHostPort(p.IP, strconv.Itoa(p.Port)))
	if err != nil {
		return err
	}

	p.Kite.Log.Notice("Listening on: %s", p.listener.Addr().String())

	kon := kontrolclient.New(p.Kite)

	if p.RegisterToKontrol {
		reg := registration.New(kon)
		p.url = &url.URL{
			Scheme: "ws",
			Host:   net.JoinHostPort(p.Domain, strconv.Itoa(p.Port)),
			Path:   "/kite",
		}
		kon.DialForever()
		go reg.RegisterToKontrol(p.url)
	}

	return http.Serve(p.listener, p.mux)
}

func (p *Proxy) Start() {
	go p.ListenAndServe()
	time.Sleep(1e9)
}

func (p *Proxy) Run() {
	p.ListenAndServe()
}

func (p *Proxy) ListenAndServeTLS(certFile, keyfile string) error {
	cert, err := tls.X509KeyPair([]byte(p.cert), []byte(p.key))
	if err != nil {
		p.Kite.Log.Fatal(err.Error())
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	p.listener, err = net.Listen("tcp", net.JoinHostPort(p.IP, strconv.Itoa(p.Port)))
	if err != nil {
		p.Kite.Log.Fatal(err.Error())
	}
	p.listener = tls.NewListener(p.listener, tlsConfig)

	// Need to update these manually. TODO FIX
	// p.Kite.URL.Scheme = "wss"
	// p.Kite.ServingURL.Scheme = "wss"

	// p.Kite.Register(p.listener.Addr())

	server := &http.Server{
		Handler:   p.mux,
		TLSConfig: tlsConfig,
	}

	return server.Serve(p.listener)
}

func (p *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	p.kites[r.RemoteKite.ID] = newPrivateKite(r.RemoteKite)

	proxyURL := url.URL{
		Scheme:   p.url.Scheme,
		Host:     p.url.Host,
		Path:     "proxy",
		RawQuery: "kiteID=" + r.RemoteKite.ID,
	}

	return proxyURL.String(), nil
}

// handleProxy is the client side of the Tunnel (on public network).
func (p *Proxy) handleProxy(ws *websocket.Conn) {
	req := ws.Request()

	kiteID := req.URL.Query().Get("kiteID")

	remoteKite, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", req.URL.String())
		return
	}

	tunnel := remoteKite.newTunnel(ws)
	defer tunnel.Close()

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	const ttl = time.Duration(1 * time.Hour)
	const leeway = time.Duration(1 * time.Minute)

	token.Claims = map[string]interface{}{
		"sub": remoteKite.ID,                                // kite ID
		"seq": tunnel.id,                                    // tunnel number
		"iat": time.Now().UTC().Unix(),                      // Issued At
		"exp": time.Now().UTC().Add(ttl).Add(leeway).Unix(), // Expiration Time
		"nbf": time.Now().UTC().Add(-leeway).Unix(),         // Not Before
	}

	signed, err := token.SignedString([]byte(p.privKey))
	if err != nil {
		p.Kite.Log.Critical("Cannot sign token: %s", err.Error())
		return
	}

	tunnelURL := *p.url
	tunnelURL.Path = "/tunnel"
	tunnelURL.RawQuery = "token=" + signed

	_, err = remoteKite.Tell("kite.tunnel", map[string]string{"url": tunnelURL.String()})
	if err != nil {
		p.Kite.Log.Error("Cannot open tunnel to the kite: %s", remoteKite.Kite)
		return
	}

	select {
	case <-tunnel.StartNotify():
		<-tunnel.CloseNotify()
	case <-time.After(1 * time.Minute):
		p.Kite.Log.Error("timeout")
	}
}

// handleTunnel is the PrivateKite side of the Tunnel (on private network).
func (p *Proxy) handleTunnel(ws *websocket.Conn) {
	tokenString := ws.Request().URL.Query().Get("token")

	getPublicKey := func(token *jwt.Token) ([]byte, error) {
		return []byte(p.pubKey), nil
	}

	token, err := jwt.Parse(tokenString, getPublicKey)
	if err != nil {
		p.Kite.Log.Error("Invalid token: \"%s\"", tokenString)
		return
	}

	kiteID := token.Claims["sub"].(string)
	seq := uint64(token.Claims["seq"].(float64))

	remoteKite, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", kiteID)
		return
	}

	tunnel, ok := remoteKite.tunnels[seq]
	if !ok {
		p.Kite.Log.Error("Tunnel not found: %d", seq)
	}

	go tunnel.Run(ws)

	<-tunnel.CloseNotify()

}

//
// PrivateKite
//

type PrivateKite struct {
	*kite.RemoteKite

	// Connections to kites behind the proxy. Keys are kite IDs.
	tunnels map[uint64]*Tunnel

	// Last tunnel number
	seq uint64
}

func newPrivateKite(r *kite.RemoteKite) *PrivateKite {
	return &PrivateKite{
		RemoteKite: r,
		tunnels:    make(map[uint64]*Tunnel),
	}
}

func (k *PrivateKite) newTunnel(local *websocket.Conn) *Tunnel {
	t := &Tunnel{
		id:        atomic.AddUint64(&k.seq, 1),
		localConn: local,
		startChan: make(chan bool),
		closeChan: make(chan bool),
	}

	// Add to map.
	k.tunnels[t.id] = t

	// Delete from map on close.
	go func() {
		<-t.CloseNotify()
		delete(k.tunnels, t.id)
	}()

	return t
}
