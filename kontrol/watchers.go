package main

import (
	"koding/newkite/dnode"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"reflect"
	"sync"
)

type watchers struct {
	sync.RWMutex
	requests map[*kite.RemoteKite][]*request
}

type request struct {
	query    *KontrolQuery
	callback dnode.Function
}

func newWatchers() *watchers {
	return &watchers{requests: make(map[*kite.RemoteKite][]*request)}
}

// RegisterWatcher saves the callback to invoke later
// when a Kite is registered matching the query.
func (w *watchers) RegisterWatcher(r *kite.RemoteKite, q *KontrolQuery, cb dnode.Function) {
	w.Lock()
	defer w.Unlock()
	r.Client.OnDisconnect(func() {
		w.Lock()
		delete(w.requests, r)
		w.Unlock()
	})
	w.requests[r] = append(w.requests[r], &request{q, cb})
}

// Notify is called when a Kite is registered.
func (w *watchers) Notify(kite *protocol.Kite) {
	w.RLock()
	defer w.RUnlock()

	// Iterating over every watch request is really not efficient.
	// However, I have written in easy way because we are going to replace
	// this functionality with Zookeeper, Etcd or similar service.
	for _, requests := range w.requests {
		for _, request := range requests {
			if matches(kite, request.query) {
				go request.callback(kite)
			}
		}
	}
}

func matches(kite *protocol.Kite, query *KontrolQuery) bool {
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
