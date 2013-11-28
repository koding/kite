package kontrol

import (
	"container/list"
	"koding/newkite/dnode"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"reflect"
	"sync"
)

// watcherHub allows watches to be registered on Kites and allows them to
// be notified when a Kite changes (registered or deregistered).
type watcherHub struct {
	sync.RWMutex

	// Indexed by user to iterate faster when a notification comes.
	// Indexed by user because it is the first field in protocol.KontrolQuery.
	watchesByUser map[string]*list.List // List contains *watch

	// Indexed by Kite to remove them easily when Kite disconnects.
	watchesByKite map[*kite.RemoteKite][]*list.Element
}

type watch struct {
	query    *protocol.KontrolQuery
	callback dnode.Function
}

func newWatcherHub() *watcherHub {
	return &watcherHub{
		watchesByUser: make(map[string]*list.List),
		watchesByKite: make(map[*kite.RemoteKite][]*list.Element),
	}
}

// RegisterWatcher saves the callbacks to invoke later
// when a Kite is registered/deregistered matching the query.
func (h *watcherHub) RegisterWatcher(r *kite.RemoteKite, q *protocol.KontrolQuery, callback dnode.Function) {
	h.Lock()
	defer h.Unlock()

	r.OnDisconnect(func() {
		h.Lock()
		defer h.Unlock()

		// Delete watch from watchesByUser
		for _, elem := range h.watchesByKite[r] {
			l := h.watchesByUser[q.Username]
			l.Remove(elem)

			// Delete the empty list.
			if l.Len() == 0 {
				delete(h.watchesByUser, q.Username)
			}
		}

		delete(h.watchesByKite, r)
	})

	// Get or create a new list.
	l, ok := h.watchesByUser[q.Username]
	if !ok {
		l = list.New()
		h.watchesByUser[q.Username] = l
	}

	elem := l.PushBack(&watch{q, callback})
	h.watchesByKite[r] = append(h.watchesByKite[r], elem)
}

// Notify is called when a Kite is registered by the user of this watcherHub.
// Calls the registered callbacks mathching to the kite.
func (h *watcherHub) Notify(kite *protocol.Kite, action protocol.KiteAction, kodingKey string) {
	h.RLock()
	defer h.RUnlock()

	l, ok := h.watchesByUser[kite.Username]
	if !ok {
		return
	}

	for e := l.Front(); e != nil; e = e.Next() {
		watch := e.Value.(*watch)
		if !matches(kite, watch.query) {
			continue
		}

		var kiteWithToken *protocol.KiteWithToken
		var err error

		// Register events needs a token attached.
		if action == protocol.Register {
			kiteWithToken, err = addTokenToKite(kite, watch.query.Username, kodingKey)
			if err != nil {
				log.Error("watch notify: %s", err)
				continue
			}

		} else {
			// We do not need to send token for deregister event.
			kiteWithToken = &protocol.KiteWithToken{Kite: *kite}
		}

		event := protocol.KiteEvent{
			Action: action,
			Kite:   kiteWithToken.Kite,
			Token:  kiteWithToken.Token,
		}
		go watch.callback(event)
	}
}

// matches returns true if kite mathches to the query.
func matches(kite *protocol.Kite, query *protocol.KontrolQuery) bool {
	qv := reflect.ValueOf(*query)
	qt := qv.Type()

	for i := 0; i < qt.NumField(); i++ {
		qf := qv.Field(i)

		// Empty field in query matches everything.
		if qf.String() == "" {
			continue
		}

		// Compare field qf. query does not match if any field is different.
		kf := reflect.ValueOf(*kite).FieldByName(qt.Field(i).Name)
		if kf.String() != qf.String() {
			return false
		}
	}

	return true
}
