package server

import (
	"crypto/tls"
	"fmt"
	"github.com/koding/kite"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Server struct {
	*kite.Kite
	listener  net.Listener
	TLSConfig *tls.Config

	// Trusted root certificates for TLS connections (wss://).
	// Certificate data must be PEM encoded.
	//tlsCertificates [][]byte

	readyC chan bool // To signal when kite is ready to accept connections
	closeC chan bool // To signal when kite is closed with Close()

	// Handlers to call when a Kite opens a connection to this Kite.
	onConnectHandlers []func(*kite.RemoteKite)

	// Handlers to call when a client has disconnected.
	onDisconnectHandlers []func(*kite.RemoteKite)

	// Should handlers run concurrently? Default is true.
	concurrent bool
}

func New(k *kite.Kite) *Server {
	return &Server{
		Kite:   k,
		readyC: make(chan bool),
		closeC: make(chan bool),
	}
}

// // Normally, each incoming request is processed in a new goroutine.
// // If you disable concurrency, requests will be processed synchronously.
// func (k *Kite) DisableConcurrency() {
// 	k.server.SetConcurrent(false)
// }

// // OnConnect registers a function to run when a Kite connects to this Kite.
// func (k *Kite) OnConnect(handler func(*RemoteKite)) {
// 	k.onConnectHandlers = append(k.onConnectHandlers, handler)
// }

// // OnDisconnect registers a function to run when a connected Kite is disconnected.
// func (k *Kite) OnDisconnect(handler func(*RemoteKite)) {
// 	k.onDisconnectHandlers = append(k.onDisconnectHandlers, handler)
// }

// // notifyRemoteKiteConnected runs the registered handlers with OnConnect().
// func (k *Kite) notifyRemoteKiteConnected(r *RemoteKite) {
// 	k.Log.Info("Client '%s' is identified as '%s'",
// 		r.client.Conn.Request().RemoteAddr, r.Name)

// 	for _, handler := range k.onConnectHandlers {
// 		go handler(r)
// 	}
// }

// // notifyRemoteKiteDisconnected runs the registered handlers with OnDisconnect().
// func (k *Kite) notifyRemoteKiteDisconnected(r *RemoteKite) {
// 	k.Log.Info("Client has disconnected: %s", r.Name)

// 	for _, handler := range k.onDisconnectHandlers {
// 		go handler(r)
// 	}
// }

// k.server.OnConnect(func(c *rpc.Client) {
// 	k.Log.Info("New connection from: %s", c.Conn.Request().RemoteAddr)
// })

// // Run OnDisconnect handlers when a client has disconnected.
// k.server.OnDisconnect(func(c *rpc.Client) {
// 	if r, ok := c.Properties()["remoteKite"]; ok {
// 		k.notifyRemoteKiteDisconnected(r.(*RemoteKite))
// 	}
// })

// // // Add new trusted root certificate for TLS from a PEM block.
// // func (k *Server) AddRootCertificate(cert string) {
// //  k.tlsCertificates = append(k.tlsCertificates, []byte(cert))
// // }

// // // Add new trusted root certificate for TLS from a file name.
// // func (k *Server) AddRootCertificateFile(certFile string) {
// //  data, err := ioutil.ReadFile(certFile)
// //  if err != nil {
// //      k.Log.Fatal("Cannot add certificate: %s", err.Error())
// //  }
// //  k.tlsCertificates = append(k.tlsCertificates, data)
// // }

// // // EnableTLS enables "wss://" protocol".
// // // It uses the same port and disables "ws://".
// // func (k *Kite) EnableTLS(certFile, keyFile string) {
// //  cert, err := tls.LoadX509KeyPair(certFile, keyFile)
// //  if err != nil {
// //      k.Log.Fatal(err.Error())
// //  }

// //  k.server.TlsConfig = &tls.Config{
// //      Certificates: []tls.Certificate{cert},
// //  }

// //  k.Kite.URL.Scheme = "wss"
// //  k.ServingURL.Scheme = "wss"
// // }

// // func (k *Kite) tlsConfig() *tls.Config {
// //         c := &tls.Config{RootCAs: x509.NewCertPool()}
// //         for _, cert := range k.tlsCertificates {
// //                 c.RootCAs.AppendCertsFromPEM(cert)
// //         }
// //         return c
// // }

func (s *Server) CloseNotify() chan bool {
	return s.closeC
}

func (s *Server) ReadyNotify() chan bool {
	return s.readyC
}

// Start is like Run(), but does not wait for it to complete. It's nonblocking.
func (s *Server) Start() {
	go s.Run()
	<-s.readyC // wait until we are ready
}

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously.
func (s *Server) Run() {
	if os.Getenv("KITE_VERSION") != "" {
		fmt.Println(s.Kite.Kite().Version)
		os.Exit(0)
	}

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	err := s.ListenAndServe()
	if err != nil {
		if strings.Contains(err.Error(), errClosing) {
			// The server is closed by Close() method
			s.Kite.Log.Notice("Kite server is closed.")
			return
		}
		s.Kite.Log.Fatal(err.Error())
	}
}

// Close stops the server.
func (s *Server) Close() {
	s.Kite.Log.Notice("Closing server...")
	s.listener.Close()
	s.Kite.Log.Close()
}

func (s *Server) Addr() string {
	return net.JoinHostPort(s.Kite.Config.IP, strconv.Itoa(s.Kite.Config.Port))
}

func (s *Server) listen() error {
	var err error
	s.listener, err = net.Listen("tcp4", s.Addr())
	if err != nil {
		return err
	}

	s.Kite.Log.Notice("Listening: %s", s.listener.Addr().String())
	return nil
}

// ListenAndServe listens on the TCP network address k.URL.Host and then
// calls Serve to handle requests on incoming connections.
func (s *Server) ListenAndServe() error {
	err := s.listen()
	if err != nil {
		return err
	}
	close(s.readyC) // listener is ready, notify waiters.
	return s.Serve(s.listener)
}

func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	config := &tls.Config{}
	if s.TLSConfig != nil {
		*config = *s.TLSConfig
	}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	err = s.listen()
	if err != nil {
		return err
	}

	s.listener = tls.NewListener(s.listener, config)
	return s.Serve(s.listener)
}

func (s *Server) Serve(l net.Listener) error {
	s.listener = l
	s.Kite.Log.Info("Serving...")
	defer close(s.closeC) // serving is finished, notify waiters.
	return http.Serve(l, s.Kite)
}
