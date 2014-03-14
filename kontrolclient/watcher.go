package kontrolclient

import (
	"container/list"
	"sync"

	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

type Watcher struct {
	id            string
	query         *protocol.KontrolQuery
	handler       EventHandler
	kontrolClient *KontrolClient
	canceled      bool
	mutex         sync.Mutex
	elem          *list.Element
}

type EventHandler func(*Event, *kite.Error)

func (k *KontrolClient) newWatcher(id string, query *protocol.KontrolQuery, handler EventHandler) *Watcher {
	watcher := &Watcher{
		id:            id,
		query:         query,
		handler:       handler,
		kontrolClient: k,
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

	_, err := w.kontrolClient.Tell("cancelWatcher", w.id)
	if err == nil {
		w.canceled = true

		// Remove from kontrolClient's watcher list.
		w.kontrolClient.watchersMutex.Lock()
		w.kontrolClient.watchers.Remove(w.elem)
		w.kontrolClient.watchersMutex.Unlock()
	}

	return err
}

func (w *Watcher) rewatch() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id, err := w.kontrolClient.watchKites(*w.query, w.handler)
	if err != nil {
		return err
	}
	w.id = id
	return nil
}
