package kontrol

import (
	"errors"

	"github.com/coreos/go-etcd/etcd"
	"github.com/hashicorp/go-version"
	"github.com/koding/kite"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
)

type Event struct {
	Action   string     `json:"action"`
	Node     *etcd.Node `json:"node,omitempty"`
	PrevNode *etcd.Node `json:"prevNode,omitempty"`
}

func (k *Kontrol) handleCancelWatcher(r *kite.Request) (interface{}, error) {
	id := r.Args.One().MustString()
	return nil, k.cancelWatcher(id)
}

func (k *Kontrol) cancelWatcher(watcherID string) error {
	k.watchersMutex.Lock()
	defer k.watchersMutex.Unlock()
	watcher, ok := k.watchers[watcherID]
	if !ok {
		return errors.New("Watcher not found")
	}
	watcher.Stop()
	delete(k.watchers, watcherID)
	return nil
}

// TODO watchAndSendKiteEvents takes too many arguments. Refactor it.
func (k *Kontrol) watchAndSendKiteEvents(
	watcher *Watcher,
	watcherID string,
	disconnect chan bool,
	etcdKey string,
	callback dnode.Function,
	token string,
	hasConstraint bool,
	constraint version.Constraints,
	keyRest string,
) {
	var index uint64 = 0
	for {
		select {
		case <-disconnect:
			return
		case resp, ok := <-watcher.recv:
			// Channel is closed. This happens in 3 cases:
			//   1. Remote kite called "cancelWatcher" method and removed the watcher.
			//   2. Remote kite has disconnected and the watcher is removed.
			//   3. Remote kite couldn't consume messages fast enough, buffer
			//      has filled up and etcd cancelled the watcher.
			if !ok {
				// Do not try again if watcher is cancelled.
				k.watchersMutex.Lock()
				if _, ok := k.watchers[watcherID]; !ok {
					k.watchersMutex.Unlock()
					return
				}

				// Do not try again if disconnected.
				select {
				case <-disconnect:
					k.watchersMutex.Unlock()
					return
				default:
				}
				k.watchersMutex.Unlock()

				// If we are here that means we did not consume fast enough and etcd
				// has canceled our watcher. We need to create a new watcher with the same key.
				var err error

				watcher, err = k.storage.Watch(KitesPrefix+etcdKey, index)
				if err != nil {
					k.Kite.Log.Error("Cannot re-watch query: %s", err.Error())
					callback.Call(kite.Response{
						Error: &kite.Error{
							Type:    "watchError",
							Message: err.Error(),
						},
					})
					return
				}

				continue
			}

			etcdEvent := &Event{
				Action:   resp.Action,
				Node:     resp.Node,
				PrevNode: resp.PrevNode,
			}

			index = etcdEvent.Node.ModifiedIndex

			switch etcdEvent.Action {
			case "set":
				// Do not send Register events for heartbeat messages.
				// PrevNode must be empty if the kite has registered for the first time.
				if etcdEvent.PrevNode != nil {
					continue
				}

				otherKite, err := NewNode(etcdEvent.Node).Kite()
				if err != nil {
					continue
				}
				otherKite.Token = token

				if hasConstraint && !isValid(&otherKite.Kite, constraint, keyRest) {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Register
				e.Kite = otherKite.Kite
				e.URL = otherKite.URL
				e.Token = otherKite.Token

				callback.Call(kite.Response{Result: e})

			// Delete happens when we detect that otherKite is disconnected.
			// Expire happens when we don't get heartbeat from otherKite.
			case "delete", "expire":
				otherKite, err := NewNode(etcdEvent.Node).KiteFromKey()
				if err != nil {
					continue
				}

				if hasConstraint && !isValid(otherKite, constraint, keyRest) {
					continue
				}

				var e protocol.KiteEvent
				e.Action = protocol.Deregister
				e.Kite = *otherKite

				callback.Call(kite.Response{Result: e})
			}
		}
	}
}
