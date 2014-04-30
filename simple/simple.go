// Package simple implements a helper for kites to serve on HTTP,
// and registration to kontrol and proxy kites.
package simple

import (
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/registration"
)

// simple kite server
type Simple struct {
	*kite.Kite
	Registration *registration.Registration
}

func New(name, version string) *Simple {
	k := kite.New(name, version)

	conf, err := config.Get()
	if err != nil {
		k.Log.Fatal("Cannot get config: %s", err.Error())
	}

	k.Config = conf
	s := &Simple{
		Kite:         k,
		Registration: registration.New(k),
	}

	return s
}

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (s *Simple) HandleFunc(method string, handler kite.HandlerFunc) {
	s.Kite.HandleFunc(method, handler)
}

func (s *Simple) Start() {
	s.Log.Info("Kite has started: %s", s.Kite.Kite())
	s.Kite.Start()
	go s.Registration.RegisterToProxyAndKontrol()
}

func (s *Simple) Run() {
	s.Kite.Start()
	<-s.CloseNotify()
}

func (s *Simple) Close() {
	s.Kontrol.Close()
	s.Kite.Close()
}
