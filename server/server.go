package server

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/koding/kite"
)

type Server struct {
	*kite.Kite
	listener  net.Listener
	TLSConfig *tls.Config
	readyC    chan bool // To signal when kite is ready to accept connections
	closeC    chan bool // To signal when kite is closed with Close()
}

func New(k *kite.Kite) *Server {
	return &Server{
		Kite:   k,
		readyC: make(chan bool),
		closeC: make(chan bool),
	}
}

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

	err := s.listenAndServe()
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
}

func (s *Server) Addr() string {
	return net.JoinHostPort(s.Kite.Config.IP, strconv.Itoa(s.Kite.Config.Port))
}

// listenAndServe listens on the TCP network address k.URL.Host and then
// calls Serve to handle requests on incoming connections.
func (s *Server) listenAndServe() error {
	var err error
	s.listener, err = net.Listen("tcp4", s.Addr())
	if err != nil {
		return err
	}

	s.Kite.Log.Notice("Listening: %s", s.listener.Addr().String())

	if s.TLSConfig != nil {
		if s.TLSConfig.NextProtos == nil {
			s.TLSConfig.NextProtos = []string{"http/1.1"}
		}
		s.listener = tls.NewListener(s.listener, s.TLSConfig)
	}

	close(s.readyC) // listener is ready, notify waiters.
	s.Kite.Log.Info("Serving...")
	defer close(s.closeC) // serving is finished, notify waiters.
	return http.Serve(s.listener, s.Kite)
}

func (k *Server) UseTLS(certPEM, keyPEM string) {
	config := &tls.Config{}
	if k.TLSConfig != nil {
		k.TLSConfig = config
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		panic(err)
	}

	config.Certificates = append(config.Certificates, cert)
}

func (k *Server) UseTLSFile(certFile, keyFile string) {
	certData, err := ioutil.ReadFile(certFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}
	keyData, err := ioutil.ReadFile(certFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}
	k.UseTLS(string(certData), string(keyData))
}
