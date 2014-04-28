package kite

import (
	"container/list"
	"sync"

	"github.com/koding/kite/protocol"
)

type Watcher struct {
	id        string
	query     *protocol.KontrolQuery
	handler   EventHandler
	localKite *Kite
	canceled  bool
	mutex     sync.Mutex
	elem      *list.Element
}

type EventHandler func(*Event, *Error)

func (k *Kite) newWatcher(id string, query *protocol.KontrolQuery, handler EventHandler) *Watcher {
	watcher := &Watcher{
		id:        id,
		query:     query,
		handler:   handler,
		localKite: k,
	}

	// Add to the kontrol's watchers list.
	k.Kontrol.watchersMutex.Lock()
	watcher.elem = k.Kontrol.watchers.PushBack(watcher)
	k.Kontrol.watchersMutex.Unlock()

	return watcher
}

func (w *Watcher) Cancel() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.canceled {
		return nil
	}

	_, err := w.localKite.Kontrol.Tell("cancelWatcher", w.id)
	if err == nil {
		w.canceled = true

		// Remove from kontrolClient's watcher list.
		w.localKite.Kontrol.watchersMutex.Lock()
		w.localKite.Kontrol.watchers.Remove(w.elem)
		w.localKite.Kontrol.watchersMutex.Unlock()
	}

	return err
}

func (w *Watcher) rewatch() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id, err := w.localKite.watchKites(*w.query, w.handler)
	if err != nil {
		return err
	}
	w.id = id
	return nil
}
