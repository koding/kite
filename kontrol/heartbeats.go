package kontrol

import (
	"errors"
	"sync"
	"time"
)

var deleteAfter = HeartbeatInterval * 2

type Heartbeats struct {
	kites map[string]*time.Timer
	sync.Mutex
}

func (h *Heartbeats) Start(id string) {
	h.Lock()
	defer h.Unlock()

	timer := time.AfterFunc(deleteAfter, func() {
		// delete key from the storage
	})

	h.kites[id] = timer
}

// Update resets the timer for the given id so it's called after the given
// duration.
func (h *Heartbeats) Update(id string, after time.Duration) error {
	h.Lock()
	defer h.Unlock()

	timer, ok := h.kites[id]
	if !ok {
		return errors.New("kite not found")
	}

	// if the timer is already fired (returns false, so the timer func is
	// called and the key is already deleted), then there is no need for the
	// timer anymore, so stop it and remove it from the map
	if active := timer.Reset(after); !active {
		timer.Stop()
		h.kites[id] = nil   // garbage collect it
		delete(h.kites, id) // the timer is already fired, so delete it from the map
	}

	return nil
}
