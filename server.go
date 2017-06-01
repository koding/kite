// Package server implements a HTTP(S) server for kites.
package kite

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously. It supports graceful restart via SIGUSR2.
func (k *Kite) Run() {
	if os.Getenv("KITE_VERSION") != "" {
		fmt.Println(k.Kite().Version)
		os.Exit(0)
	}

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	err := k.listenAndServe()
	if err != nil {
		if strings.Contains(err.Error(), errClosing) {
			// The server is closed by Close() method
			k.Log.Info("Kite server is closed.")
			return
		}
		k.Log.Fatal(err.Error())
	}
}

// Close stops the server and the kontrol client instance.
func (k *Kite) Close() {
	k.Log.Info("Closing kite...")

	k.kontrol.Lock()
	if k.kontrol != nil && k.kontrol.Client != nil {
		k.kontrol.Close()
	}
	k.kontrol.Unlock()

	if k.listener != nil {
		k.listener.Close()
		k.listener = nil
	}

	k.mu.Lock()
	cache := k.verifyCache
	k.mu.Unlock()

	if cache != nil {
		cache.StopGC()
	}
}

func (k *Kite) Addr() string {
	return net.JoinHostPort(k.Config.IP, strconv.Itoa(k.Config.Port))
}

// listenAndServe listens on the TCP network address k.URL.Host and then
// calls Serve to handle requests on incoming connectionk.
func (k *Kite) listenAndServe() error {
	// create a new one if there doesn't exist
	l, err := net.Listen("tcp4", k.Addr())
	if err != nil {
		return err
	}

	k.Log.Info("New listening: %s", l.Addr())

	if k.TLSConfig != nil {
		if k.TLSConfig.NextProtos == nil {
			k.TLSConfig.NextProtos = []string{"http/1.1"}
		}
		l = tls.NewListener(l, k.TLSConfig)
	}

	k.listener = newGracefulListener(l)

	// listener is ready, notify waiters.
	close(k.readyC)

	defer close(k.closeC) // serving is finished, notify waiters.
	k.Log.Info("Serving...")

	return k.serve(k.listener, k)
}

func (k *Kite) serve(l net.Listener, h http.Handler) error {
	if k.Config.Serve != nil {
		return k.Config.Serve(l, h)
	}
	return http.Serve(l, h)
}

// Port returns the TCP port number that the kite listens.
// Port must be called after the listener is initialized.
// You can use ServerReadyNotify function to get notified when listener is ready.
//
// Kite starts to listen the port when Run() is called.
// Since Run() is blocking you need to run it as a goroutine the call this function when listener is ready.
//
// Example:
//
//   k := kite.New("x", "1.0.0")
//   go k.Run()
//   <-k.ServerReadyNotify()
//   port := k.Port()
//
func (k *Kite) Port() int {
	if k.listener == nil {
		return 0
	}

	return k.listener.Addr().(*net.TCPAddr).Port
}

func (k *Kite) UseTLS(certPEM, keyPEM string) {
	if k.TLSConfig == nil {
		k.TLSConfig = &tls.Config{}
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		panic(err)
	}

	k.TLSConfig.Certificates = append(k.TLSConfig.Certificates, cert)
}

func (k *Kite) UseTLSFile(certFile, keyFile string) {
	certData, err := ioutil.ReadFile(certFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}

	keyData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}

	k.UseTLS(string(certData), string(keyData))
}

func (k *Kite) ServerCloseNotify() chan bool {
	return k.closeC
}

func (k *Kite) ServerReadyNotify() chan bool {
	return k.readyC
}

// gracefulListener closes all accepted connections upon Close to ensure
// no dangling websocket/xhr sessions outlive the kite.
type gracefulListener struct {
	net.Listener

	conns   map[net.Conn]struct{}
	connsMu sync.Mutex
}

func newGracefulListener(l net.Listener) *gracefulListener {
	return &gracefulListener{
		Listener: l,
		conns:    make(map[net.Conn]struct{}),
	}
}

func (l *gracefulListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	l.connsMu.Lock()
	l.conns[conn] = struct{}{}
	l.connsMu.Unlock()

	return &gracefulConn{
		Conn: conn,
		close: func() {
			l.connsMu.Lock()
			delete(l.conns, conn)
			l.connsMu.Unlock()
		},
	}, nil
}

func (l *gracefulListener) Close() error {
	err := l.Listener.Close()

	l.connsMu.Lock()
	for conn := range l.conns {
		conn.Close()
	}
	l.conns = nil
	l.connsMu.Unlock()

	return err
}

type gracefulConn struct {
	net.Conn

	close func()
}

func (c *gracefulConn) Close() error {
	c.close()

	return c.Conn.Close()
}
