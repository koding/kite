// Package simple implements a helper for kites to serve on HTTP,
// and registration to kontrol and proxy kites.
package simple

import (
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/registration"
	"github.com/koding/kite/server"
)

// simple kite server
type Simple struct {
	*server.Server
	LocalKite    *kite.Kite
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

	s := &Simple{
		Server:       server,
		LocalKite:    k,
		Registration: registration.New(k),
	}

	return s
}

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (s *Simple) HandleFunc(method string, handler kite.HandlerFunc) {
	s.Server.Kite.HandleFunc(method, handler)
}

func (s *Simple) Start() {
	s.Log.Info("Kite has started: %s", s.Kite.Kite())
	s.Server.Start()
	go s.Registration.RegisterToProxyAndKontrol()
}

func (s *Simple) Run() {
	s.Start()
	<-s.Server.CloseNotify()
}

func (s *Simple) Close() {
	s.LocalKite.Kontrol.Close()
	s.Server.Close()
}
