// Package tunnelproxy implements a reverse-proxy for kites behind firewall or NAT.
package tunnelproxy

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"

	"github.com/dgrijalva/jwt-go"
	"github.com/igm/sockjs-go/sockjs"
)

const (
	ProxyVersion = "0.0.2"
)

var (
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
	k := kite.New("tunnelproxy", version)
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

	p.mux.Handle("/", p.Kite)
	p.mux.Handle("/proxy/", sockjsHandlerWithRequest("/proxy", sockjs.DefaultOptions, p.handleProxy))    // Handler for clients outside
	p.mux.Handle("/tunnel/", sockjsHandlerWithRequest("/tunnel", sockjs.DefaultOptions, p.handleTunnel)) // Handler for kites behind

	// Remove URL from the map when PrivateKite disconnects.
	k.OnDisconnect(func(r *kite.Client) {
		delete(p.kites, r.Kite.ID)
	})

	return p
}

// sockjsHandlerWithRequest is a wrapper around the sockjs.Handler that
// includes a *http.Request context.
func sockjsHandlerWithRequest(
	prefix string,
	opts sockjs.Options,
	handleFunc func(sockjs.Session, *http.Request),
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sockjs.NewHandler(prefix, opts, func(session sockjs.Session) {
			handleFunc(session, r)
		}).ServeHTTP(w, r)
	})
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
		Scheme:   "http",
		Host:     p.url.Host,
		Path:     "proxy",
		RawQuery: "kiteID=" + r.Client.ID,
	}

	return proxyURL.String(), nil
}

// handleProxy is the client side of the Tunnel (on public network).
func (p *Proxy) handleProxy(session sockjs.Session, req *http.Request) {
	const ttl = time.Duration(1 * time.Hour)
	const leeway = time.Duration(1 * time.Minute)

	kiteID := req.URL.Query().Get("kiteID")

	client, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", req.URL.String())
		return
	}

	// TODO(rjeczalik): keep *rsa.PrivateKey in Proxy struct
	rsaPrivate, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(p.privKey))
	if err != nil {
		p.Kite.Log.Error("key pair encrypt error: %s", err)
		return
	}

	tunnel := client.newTunnel(session)
	defer tunnel.Close()

	claims := jwt.MapClaims{
		"sub": client.ID,                                    // kite ID
		"seq": tunnel.id,                                    // tunnel number
		"iat": time.Now().UTC().Unix(),                      // Issued At
		"exp": time.Now().UTC().Add(ttl).Add(leeway).Unix(), // Expiration Time
		"nbf": time.Now().UTC().Add(-leeway).Unix(),         // Not Before
	}

	signed, err := jwt.NewWithClaims(jwt.GetSigningMethod("RS256"), claims).SignedString(rsaPrivate)
	if err != nil {
		p.Kite.Log.Error("Cannot sign token: %s", err.Error())
		return
	}

	tunnelURL := *p.url
	tunnelURL.Path = "/tunnel" + strings.TrimPrefix(req.URL.Path, "/proxy")
	tunnelURL.RawQuery = "token=" + signed

	_, err = client.TellWithTimeout("kite.tunnel",
		4*time.Second, map[string]string{"url": tunnelURL.String()})
	if err != nil {
		p.Kite.Log.Error("Cannot open tunnel to the kite: %s err: %s", client.Kite, err.Error())
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
func (p *Proxy) handleTunnel(session sockjs.Session, req *http.Request) {
	tokenString := req.URL.Query().Get("token")

	getPublicKey := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("invalid signing method")
		}

		return jwt.ParseRSAPublicKeyFromPEM([]byte(p.pubKey))
	}

	token, err := jwt.Parse(tokenString, getPublicKey)
	if err != nil {
		p.Kite.Log.Error("Invalid token: \"%s\"", tokenString)
		return
	}

	kiteID := token.Claims.(jwt.MapClaims)["sub"].(string)
	seq := uint64(token.Claims.(jwt.MapClaims)["seq"].(float64))

	client, ok := p.kites[kiteID]
	if !ok {
		p.Kite.Log.Error("Remote kite is not found: %s", kiteID)
		return
	}

	tunnel, ok := client.tunnels[seq]
	if !ok {
		p.Kite.Log.Error("Tunnel not found: %d", seq)
	}

	go tunnel.Run(session)

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

func (k *PrivateKite) newTunnel(local sockjs.Session) *Tunnel {
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
