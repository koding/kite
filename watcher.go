package kite

import (
	"container/list"
	"sync"
	"time"

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
	k.kontrol.watchersMutex.Lock()
	watcher.elem = k.kontrol.watchers.PushBack(watcher)
	k.kontrol.watchersMutex.Unlock()

	return watcher
}

func (w *Watcher) Cancel() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.canceled {
		return nil
	}

	_, err := w.localKite.kontrol.TellWithTimeout("cancelWatcher", 4*time.Second, w.id)
	if err == nil {
		w.canceled = true

		// Remove from kontrolClient's watcher list.
		w.localKite.kontrol.watchersMutex.Lock()
		w.localKite.kontrol.watchers.Remove(w.elem)
		w.localKite.kontrol.watchersMutex.Unlock()
	}

	return err
}

func (w *Watcher) rewatch() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id, err := w.localKite.watchKites(w.query, w.handler)
	if err != nil {
		return err
	}
	w.id = id
	return nil
}
