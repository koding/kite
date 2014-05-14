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
)

const (
	ProxyVersion      = "0.0.2"
	DefaultPort       = 3999
	DefaultPublicHost = "localhost:3999"
)

type Proxy struct {
	Kite *kite.Kite

	listener  net.Listener
	TLSConfig *tls.Config

	readyC chan bool // To signal when kite is ready to accept connections
	closeC chan bool // To signal when kite is closed with Close()

	// If givent it must match the domain in certificate.
	PublicHost string

	// For generating token tokens for tunnels.
	pubKey  string
	privKey string

	// Holds registered kites. Keys are kite IDs.
	kites map[string]*PrivateKite

	mux *http.ServeMux

	RegisterToKontrol bool

	url *url.URL
}

func New(conf *config.Config, version, pubKey, privKey string) *Proxy {
	k := kite.New("proxy", version)
	k.Config = conf

	// Listen on 3999 by default
	if k.Config.Port == 0 {
		k.Config.Port = DefaultPort
	}

	p := &Proxy{
		Kite:              k,
		readyC:            make(chan bool),
		closeC:            make(chan bool),
		pubKey:            pubKey,
		privKey:           privKey,
		kites:             make(map[string]*PrivateKite),
		mux:               http.NewServeMux(),
		RegisterToKontrol: true,
		PublicHost:        DefaultPublicHost,
	}

	p.Kite.HandleFunc("register", p.handleRegister)

	p.mux.Handle("/kite", p.Kite)
	p.mux.Handle("/proxy", websocket.Server{Handler: p.handleProxy})   // Handler for clients outside
	p.mux.Handle("/tunnel", websocket.Server{Handler: p.handleTunnel}) // Handler for kites behind

	// Remove URL from the map when PrivateKite disconnects.
	k.OnDisconnect(func(r *kite.Client) {
		delete(p.kites, r.Kite.ID)
	})

	return p
}

func (s *Proxy) CloseNotify() chan bool {
	return s.closeC
}

func (s *Proxy) ReadyNotify() chan bool {
	return s.readyC
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

func (p *Proxy) Start() {
	go p.Run()
	<-p.readyC
}

func (p *Proxy) Run() {
	p.listenAndServe()
}

func (p *Proxy) listenAndServe() error {
	var err error
	p.listener, err = net.Listen("tcp", net.JoinHostPort(p.Kite.Config.IP, strconv.Itoa(p.Kite.Config.Port)))
	if err != nil {
		return err
	}

	p.Kite.Log.Info("Listening on: %s", p.listener.Addr().String())

	close(p.readyC)

	p.url = &url.URL{
		Scheme: "ws",
		Host:   p.PublicHost,
		Path:   "/kite",
	}

	if p.RegisterToKontrol {
		go p.Kite.RegisterForever(p.url)
	}

	defer close(p.closeC)
	return http.Serve(p.listener, p.mux)
}

func (p *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	p.kites[r.Client.ID] = newPrivateKite(r.Client)

	proxyURL := url.URL{
		Scheme:   p.url.Scheme,
		Host:     p.url.Host,
		Path:     "proxy",
		RawQuery: "kiteID=" + r.Client.ID,
	}

	return proxyURL.String(), nil
}

// handleProxy is the client side of the Tunnel (on public network).
func (p *Proxy) handleProxy(ws *websocket.Conn) {
	req := ws.Request()

	kiteID := req.URL.Query().Get("kiteID")

	client, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", req.URL.String())
		return
	}

	tunnel := client.newTunnel(ws)
	defer tunnel.Close()

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	const ttl = time.Duration(1 * time.Hour)
	const leeway = time.Duration(1 * time.Minute)

	token.Claims = map[string]interface{}{
		"sub": client.ID,                                    // kite ID
		"seq": tunnel.id,                                    // tunnel number
		"iat": time.Now().UTC().Unix(),                      // Issued At
		"exp": time.Now().UTC().Add(ttl).Add(leeway).Unix(), // Expiration Time
		"nbf": time.Now().UTC().Add(-leeway).Unix(),         // Not Before
	}

	signed, err := token.SignedString([]byte(p.privKey))
	if err != nil {
		p.Kite.Log.Error("Cannot sign token: %s", err.Error())
		return
	}

	tunnelURL := *p.url
	tunnelURL.Path = "/tunnel"
	tunnelURL.RawQuery = "token=" + signed

	_, err = client.TellWithTimeout("kite.tunnel", 4*time.Second, map[string]string{"url": tunnelURL.String()})
	if err != nil {
		p.Kite.Log.Error("Cannot open tunnel to the kite: %s", client.Kite)
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

	client, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", kiteID)
		return
	}

	tunnel, ok := client.tunnels[seq]
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
	*kite.Client

	// Connections to kites behind the proxy. Keys are kite IDs.
	tunnels map[uint64]*Tunnel

	// Last tunnel number
	seq uint64
}

func newPrivateKite(r *kite.Client) *PrivateKite {
	return &PrivateKite{
		Client:  r,
		tunnels: make(map[uint64]*Tunnel),
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
