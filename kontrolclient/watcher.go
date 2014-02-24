package kontrolclient

import (
	"container/list"
	"sync"

	"github.com/koding/kite/protocol"
)

type Watcher struct {
	id       string
	query    *protocol.KontrolQuery
	handler  EventHandler
	kontrol  *Kontrol
	canceled bool
	mutex    sync.Mutex
	elem     *list.Element
}

type EventHandler func(*Event, error)

func (k *Kontrol) newWatcher(id string, query *protocol.KontrolQuery, handler EventHandler) *Watcher {
	watcher := &Watcher{
		id:      id,
		query:   query,
		handler: handler,
		kontrol: k,
	}

	// Add to the kontrol's watchers list.
	k.watchersMutex.Lock()
	watcher.elem = k.watchers.PushBack(watcher)
	k.watchersMutex.Unlock()

	return watcher
}

func (w *Watcher) Cancel() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.canceled {
		return nil
	}

	_, err := w.kontrol.Tell("cancelWatcher", w.id)
	if err == nil {
		w.canceled = true

		// Remove from kontrol's watcher list.
		w.kontrol.watchersMutex.Lock()
		w.kontrol.watchers.Remove(w.elem)
		w.kontrol.watchersMutex.Unlock()
	}

	return err
}

func (w *Watcher) rewatch() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id, err := w.kontrol.watchKites(*w.query, w.handler)
	if err != nil {
		return err
	}
	w.id = id
	return nil
}
