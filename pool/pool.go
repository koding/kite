// Package pool implements a kite pool for staying connected to all matching
// kites in a query.
package pool

import (
	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

// Pool is helper for staying connected to every kite in a query.
type Pool struct {
	localKite *kite.Kite
	query     protocol.KontrolQuery
	Kites     map[string]*kite.Client
}

func New(k *kite.Kite, q protocol.KontrolQuery) *Pool {
	k.SetupKontrolClient()
	return &Pool{
		localKite: k,
		query:     q,
		Kites:     make(map[string]*kite.Client),
	}
}

// Start the pool (unblocking).
func (p *Pool) Start() chan error {
	errChan := make(chan error, 1)
	go func() { errChan <- p.Run() }()
	return errChan
}

// Run the pool (blocking).
func (p *Pool) Run() error {
	_, err := p.localKite.WatchKites(p.query, func(event *kite.Event, err *kite.Error) {
		switch event.Action {
		case protocol.Register:
			p.Kites[event.Kite.ID] = event.Client()
			go p.Kites[event.Kite.ID].DialForever()
		case protocol.Deregister:
			delete(p.Kites, event.Kite.ID)
		}
	})
	return err
}
