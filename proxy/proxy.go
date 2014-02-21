// Package proxy implements a reverse-proxy for kites behind firewall or NAT.
package proxy

import (
	"crypto/tls"
	"github.com/koding/kite"
	"github.com/koding/kite/util"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/dgrijalva/jwt-go"
)

type Proxy struct {
	kite *kite.Kite

	listener net.Listener

	// TLS certificate
	key  string
	cert string

	// Must match the name in certificate.
	domain string

	// For generating token tokens for tunnels.
	pubKey  string
	privKey string

	// Holds registered kites. Keys are kite IDs.
	kites map[string]*PrivateKite
}

func New(kiteOptions *kite.Options, domain string, certPEM, keyPEM string, pubKey, privKey string) *Proxy {
	proxyKite := &Proxy{
		kite:    kite.New(kiteOptions),
		domain:  domain,
		cert:    certPEM,
		key:     keyPEM,
		pubKey:  pubKey,
		privKey: privKey,
		kites:   make(map[string]*PrivateKite),
	}

	proxyKite.kite.HandleFunc("register", proxyKite.handleRegister)

	// Remove URL from the map when PrivateKite disconnects.
	proxyKite.kite.OnDisconnect(func(r *kite.RemoteKite) { delete(proxyKite.kites, r.Kite.ID) })

	return proxyKite
}

func (p *Proxy) Close() {
	p.listener.Close()
}

func (p *Proxy) ListenAndServe() error {
	cert, err := tls.X509KeyPair([]byte(p.cert), []byte(p.key))
	if err != nil {
		p.kite.Log.Fatal(err.Error())
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	mux := http.NewServeMux()
	mux.Handle(p.kite.URL.Path, p.kite)
	mux.Handle("/proxy", websocket.Server{Handler: p.handleProxy})   // Handler for clients outside
	mux.Handle("/tunnel", websocket.Server{Handler: p.handleTunnel}) // Handler for kites behind

	p.listener, err = net.Listen("tcp", p.kite.URL.Host)
	if err != nil {
		p.kite.Log.Fatal(err.Error())
	}
	p.listener = tls.NewListener(p.listener, tlsConfig)

	// Need to update these manually. TODO FIX
	p.kite.URL.Scheme = "wss"
	p.kite.ServingURL.Scheme = "wss"

	p.kite.Register(p.listener.Addr())

	server := &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	return server.Serve(p.listener)
}

func (p *Proxy) handleRegister(r *kite.Request) (interface{}, error) {
	p.kites[r.RemoteKite.ID] = newPrivateKite(r.RemoteKite)

	proxyURL := url.URL{
		Scheme:   p.kite.URL.Scheme,
		Host:     p.kite.URL.Host,
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
		p.kite.Log.Error("Remote kite is not found: %s", req.URL.String())
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
		p.kite.Log.Critical("Cannot sign token: %s", err.Error())
		return
	}

	tunnelURL := *p.kite.URL
	tunnelURL.Path = "/tunnel"
	tunnelURL.RawQuery = "token=" + signed

	_, err = remoteKite.Tell("tunnel", map[string]string{"url": tunnelURL.String()})
	if err != nil {
		p.kite.Log.Error("Cannot open tunnel to the kite: %s", remoteKite.Key())
		return
	}

	select {
	case <-tunnel.StartNotify():
		<-tunnel.CloseNotify()
	case <-time.After(1 * time.Minute):
		p.kite.Log.Error("timeout")
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
		p.kite.Log.Error("Invalid token: \"%s\"", tokenString)
		return
	}

	kiteID := token.Claims["sub"].(string)
	seq := uint64(token.Claims["seq"].(float64))

	remoteKite, ok := p.kites[kiteID]
	if !ok {
		p.kite.Log.Error("Remote kite is not found: %s", kiteID)
		return
	}

	tunnel, ok := remoteKite.tunnels[seq]
	if !ok {
		p.kite.Log.Error("Tunnel not found: %d", seq)
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

//
// Tunnel
//

type Tunnel struct {
	id          uint64          // key in kites's tunnels map
	localConn   *websocket.Conn // conn to local kite
	startChan   chan bool       // to signal started state
	closeChan   chan bool       // to signal closed state
	closed      bool            // to prevent closing closeChan again
	closedMutex sync.Mutex      // for protection of closed field
}

func (t *Tunnel) Close() {
	t.closedMutex.Lock()
	defer t.closedMutex.Unlock()

	if t.closed {
		return
	}

	close(t.closeChan)
	t.closed = true
}

func (t *Tunnel) CloseNotify() chan bool {
	return t.closeChan
}

func (t *Tunnel) StartNotify() chan bool {
	return t.startChan
}

func (t *Tunnel) Run(remoteConn *websocket.Conn) {
	close(t.startChan)
	<-util.JoinStreams(t.localConn, remoteConn)
	t.Close()
}
