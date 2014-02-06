package pool

import (
	"kite"
	"kite/protocol"
)

// Pool is helper for staying connected to every kite in a query.
type Pool struct {
	kontrol *kite.Kontrol
	query   protocol.KontrolQuery
	Kites   map[string]*kite.RemoteKite
}

func New(kontrol *kite.Kontrol, query protocol.KontrolQuery) *Pool {
	return &Pool{
		kontrol: kontrol,
		query:   query,
		Kites:   make(map[string]*kite.RemoteKite),
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
	return p.kontrol.WatchKites(p.query, func(event *kite.Event) {
		switch event.Action {
		case protocol.Register:
			p.Kites[event.Kite.ID] = event.RemoteKite()
			go p.Kites[event.Kite.ID].DialForever()
		case protocol.Deregister:
			delete(p.Kites, event.Kite.ID)
		}
	})
}
