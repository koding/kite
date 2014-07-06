// Package pool implements a kite pool for staying connected to all matching
// kites in a query.
package pool

import (
	"errors"
	"sync"

	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

var ErrNotFound = errors.New("not found")

// Pool is helper for staying connected to every kite in a query.
type Pool struct {
	localKite  *kite.Kite
	query      protocol.KontrolQuery
	kites      map[string]*kite.Client
	sync.Mutex // protects Kites map
}

func New(k *kite.Kite, q protocol.KontrolQuery) *Pool {
	return &Pool{
		localKite: k,
		query:     q,
		kites:     make(map[string]*kite.Client),
	}
}

// Start the pool (unblocking).
func (p *Pool) Start() chan error {
	errChan := make(chan error, 1)
	go func() { errChan <- p.Run() }()
	return errChan
}

// Len returns the number of total connected kites in the pool.
func (p *Pool) Len() int {
	p.Lock()
	defer p.Unlock()

	return len(p.kites)
}

// Get returns a random connect kite from the pool.
func (p *Pool) Get() (*kite.Client, error) {
	p.Lock()
	defer p.Unlock()

	// maps in go are unsorted by default. We just return the first kite we
	// got.
	for _, k := range p.kites {
		return k, nil
	}

	return nil, ErrNotFound
}

// Run the pool (blocking).
func (p *Pool) Run() error {
	_, err := p.localKite.WatchKites(p.query, func(event *kite.Event, err *kite.Error) {
		switch event.Action {
		case protocol.Register:
			p.Lock()
			p.kites[event.Kite.ID] = event.Client()
			go p.kites[event.Kite.ID].DialForever()
			p.Unlock()
		case protocol.Deregister:
			p.Lock()
			delete(p.kites, event.Kite.ID)
			p.Unlock()
		}
	})
	return err
}
