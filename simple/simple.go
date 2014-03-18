package simple

import (
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/registration"
	"github.com/koding/kite/server"
)

// simple kite server
type Simple struct {
	*server.Server
	Kontrol      *kontrolclient.KontrolClient
	Registration *registration.Registration
}

func New(name, version string) *Simple {
	k := kite.New(name, version)

	conf, err := config.Get()
	if err != nil {
		k.Log.Fatal("Cannot get config: %s", err.Error())
	}

	k.Config = conf

	server := server.New(k)

	kon := kontrolclient.New(k)

	s := &Simple{
		Server:       server,
		Kontrol:      kon,
		Registration: registration.New(kon),
	}

	return s
}

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (s *Simple) HandleFunc(method string, handler kite.HandlerFunc) {
	s.Server.Kite.HandleFunc(method, handler)
}

func (s *Simple) Start() {
	s.Log.Info("Kite has started: %s", s.Kite.Kite())
	connected, err := s.Kontrol.DialForever()
	if err != nil {
		s.Server.Log.Fatal("Cannot dial kontrol: %s", err.Error())
	}
	s.Server.Start()
	go func() {
		<-connected
		s.Registration.RegisterToProxyAndKontrol()
	}()
}

func (s *Simple) Run() {
	s.Start()
	<-s.Server.CloseNotify()
}

func (s *Simple) Close() {
	s.Kontrol.Close()
	s.Server.Close()
}
