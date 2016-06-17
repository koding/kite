package dnode

import "sync"

type Scrubber struct {
	// Next callback number.
	// Incremented atomically by register().
	seq uint64

	// Reference to sent callbacks are saved in this map.
	sync.Mutex // protects
	callbacks  map[uint64]func(*Partial)
}

// New returns a pointer to a new Scrubber.
func NewScrubber() *Scrubber {
	return &Scrubber{
		callbacks: make(map[uint64]func(*Partial)),
	}
}

// RemoveCallback removes the callback with id from callbacks.
// Can be used to remove unused callbacks to free memory.
func (s *Scrubber) RemoveCallback(id uint64) {
	s.Lock()
	delete(s.callbacks, id)
	s.Unlock()
}

func (s *Scrubber) GetCallback(id uint64) func(*Partial) {
	s.Lock()
	fn := s.callbacks[id]
	s.Unlock()
	return fn
}
