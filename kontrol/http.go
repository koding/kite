package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

func (k *Kontrol) HandleHeartbeat(rw http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("id")
	if id == "" {
		http.Error(rw, "query id is empty", http.StatusBadRequest)
		return
	}

	k.heartbeatsMu.Lock()
	defer k.heartbeatsMu.Unlock()

	k.log.Debug("Heartbeat received '%s'", id)
	if h, ok := k.heartbeats[id]; ok {
		// try to reset the timer every time the remote kite sends us a
		// heartbeat. Because the timer get reset, the timer is never fired, so
		// the value get always updated with the updater in the background
		// according to the write interval. If the kite doesn't send any
		// heartbeat, the timer func is being called, which stops the updater
		// so the key is being deleted automatically via the TTL mechanism.
		h.timer.Reset(HeartbeatInterval + HeartbeatDelay)

		k.log.Debug("Sending pong '%s'", id)
		rw.Write([]byte("pong"))
		return
	}

	// if we reach here than it has several meanings:
	// * kite was registered before, but kontrol is restarted
	// * kite was registered before, but kontrol has lost track
	// * kite was no registered and someone else sends an heartbeat
	// we send back "registeragain" so the caller can be added in to the
	// heartbeats map above.
	k.log.Debug("Sending registeragain '%s'", id)
	rw.Write([]byte("registeragain"))
}

func (k *Kontrol) HandleRegisterHTTP(rw http.ResponseWriter, req *http.Request) {
	var args protocol.RegisterArgs

	if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
		errMsg := fmt.Errorf("wrong register input: '%s'", err)
		http.Error(rw, jsonError(errMsg), http.StatusBadRequest)
		return
	}

	k.log.Info("Register (via HTTP) request from: %s", args.Kite)

	// Only accept requests with kiteKey, because that's the only way one can
	// register itself to kontrol.
	if args.Auth.Type != "kiteKey" {
		err := fmt.Errorf("unexpected authentication type: %s", args.Auth.Type)
		http.Error(rw, jsonError(err), http.StatusBadRequest)
		return
	}

	// empty url is useless for us
	if args.URL == "" {
		err := errors.New("empty URL")
		http.Error(rw, jsonError(err), http.StatusBadRequest)
		return
	}

	// decode and authenticated the token key. We'll get the authenticated
	// username
	username, err := k.Kite.AuthenticateSimpleKiteKey(args.Auth.Key)
	if err != nil {
		http.Error(rw, jsonError(err), http.StatusUnauthorized)
		return
	}
	args.Kite.Username = username

	ex := &kitekey.Extractor{
		Claims: &kitekey.KiteClaims{},
	}

	t, err := jwt.ParseWithClaims(args.Auth.Key, ex.Claims, ex.Extract)
	if err != nil {
		http.Error(rw, jsonError(err), http.StatusBadRequest)
		return
	}

	var keyPair *KeyPair
	resp := &protocol.RegisterResult{
		URL:               args.URL,
		HeartbeatInterval: int64(HeartbeatInterval / time.Second),
	}

	// check if the key is valid and is stored in the key pair storage, if not
	// found we don't allow to register anyone.
	r := &kite.Request{
		Username: username,
		Auth: &kite.Auth{
			Type: args.Auth.Type,
			Key:  args.Auth.Key,
		},
	}

	keyPair, resp.KiteKey, err = k.getOrUpdateKeyPub(ex.Claims.KontrolKey, t, r)
	if err != nil {
		http.Error(rw, jsonError(err), http.StatusBadRequest)
		return
	}

	if ex.Claims.KontrolKey != keyPair.Public {
		// NOTE(rjeczalik): updates public key for old kites, new kites
		// expect kite key to be updated
		resp.PublicKey = keyPair.Public
	}

	remoteKite := args.Kite

	// Be sure we have a valid Kite representation. We should not allow someone
	// with an empty field to be registered.
	if err := validateKiteKey(remoteKite); err != nil {
		http.Error(rw, jsonError(err), http.StatusBadRequest)
		return
	}

	// This will be stored into the final storage
	value := &kontrolprotocol.RegisterValue{
		URL:   args.URL,
		KeyID: keyPair.ID,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Upsert(remoteKite, value); err != nil {
		k.log.Error("storage add '%s' error: %s", remoteKite, err)
		http.Error(rw, jsonError(errors.New("internal error - register")), http.StatusInternalServerError)
		return
	}

	k.heartbeatsMu.Lock()
	defer k.heartbeatsMu.Unlock()

	h, ok := k.heartbeats[remoteKite.ID]
	if ok {
		// there is already a previous registration, use it
		k.log.Info("Kite was already register (via HTTP), use timer cache %s", remoteKite)

		h.timer.Reset(HeartbeatInterval + HeartbeatDelay)

		// update registerURL of the previously started heartbeat goroutine
		// so it does not get overwritten back to the old value
		h.updateC <- func() error {
			return k.storage.Update(remoteKite, value)
		}
	} else {
		// we create a new ticker which is going to update the key periodically in
		// the storage so it's always up to date. Instead of updating the key
		// periodically according to the HeartBeatInterval below, we are buffering
		// the write speed here with the UpdateInterval.
		h = &heartbeat{
			updateC: make(chan func() error),
		}

		updater := time.NewTicker(UpdateInterval)

		go func() {
			update := func() error {
				return k.storage.Update(remoteKite, value)
			}

			for {
				select {
				case <-k.closed:
					return
				case <-updater.C:
					k.log.Debug("Kite is active (via HTTP), updating the value %s", remoteKite)

					if err := update(); err != nil {
						k.log.Error("storage update '%s' error: %s", remoteKite, err)
					}
				case fn, ok := <-h.updateC:
					if !ok {
						k.log.Info("Kite is nonactive (via HTTP). Updater is closed %s", remoteKite)
						return
					}

					update = fn
				}
			}
		}()

		// we are now creating a timer that is going to call the function which
		// stops the background updater if it's not resetted. The time is being
		// resetted on a separate HTTP endpoint "/heartbeat"
		h.timer = time.AfterFunc(HeartbeatInterval+HeartbeatDelay, func() {
			k.log.Info("Kite didn't sent any heartbeat (via HTTP). Stopping the updater %s", remoteKite)

			// stop the updater so it doesn't update it in the background
			updater.Stop()

			k.heartbeatsMu.Lock()
			defer k.heartbeatsMu.Unlock()

			select {
			case <-h.updateC:
			default:
				close(h.updateC)
			}

			delete(k.heartbeats, remoteKite.ID)
		})

		k.heartbeats[remoteKite.ID] = h
	}

	k.log.Info("Kite registered (via HTTP): %s", remoteKite)

	// send the response back to the requester
	if err := json.NewEncoder(rw).Encode(resp); err != nil {
		errMsg := fmt.Errorf("could not encode response: '%s'", err)
		http.Error(rw, jsonError(errMsg), http.StatusInternalServerError)
		return
	}
}

// jsonError returns a JSON string of form {"err" : "error content"}
func jsonError(err error) string {
	var errMsg struct {
		Err string `json:"err"`
	}
	errMsg.Err = err.Error()

	data, _ := json.Marshal(&errMsg)
	return string(data)
}
