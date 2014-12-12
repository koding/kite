package kontrol

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kontrol/onceevery"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

func (k *Kontrol) handleHeartbeat(rw http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("id")

	k.heartbeatsMu.Lock()
	defer k.heartbeatsMu.Unlock()

	k.log.Info("Heartbeat received '%s'", id)
	if updateTimer, ok := k.heartbeats[id]; ok {
		// try to reset the timer every time the remote kite sends sends us a
		// heartbeat. Because the timer get reset, the timer is never fired, so
		// the value get always updated with the updater in the background
		// according to the write interval. If the kite doesn't send any
		// heartbeat, the timer func is being called, which stops the updater
		// so the key is being deleted automatically via the TTL mechanism.
		updateTimer.Reset(HeartbeatInterval + HeartbeatDelay)
		k.heartbeats[id] = updateTimer

		k.log.Info("Sending pong '%s'", id)
		rw.Write([]byte("pong"))
		return
	}

	// if we reach here than it has several meanings:
	// * kite was registered before, but kontrol is restarted
	// * kite was registered before, but kontrol has lost track
	// * kite was no registered and someone else sends an heartbeat
	// we send back "registeragain" so the caller can be added in to the
	// heartbeats map above.
	k.log.Info("Sending registeragain '%s'", id)
	rw.Write([]byte("registeragain"))
}

func (k *Kontrol) handleRegisterHTTP(r *kite.Request) (interface{}, error) {
	k.log.Info("Register (via HTTP) request from: %s", r.Client.Kite)

	if r.Args.One().MustMap()["url"].MustString() == "" {
		return nil, errors.New("invalid url")
	}

	var args struct {
		URL string `json:"url"`
	}
	r.Args.One().MustUnmarshal(&args)
	if args.URL == "" {
		return nil, errors.New("empty url")
	}

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Auth.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Auth.Type)
	}

	kiteURL := args.URL
	remote := r.Client

	if err := validateKiteKey(&remote.Kite); err != nil {
		return nil, err
	}

	value := &kontrolprotocol.RegisterValue{
		URL: kiteURL,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Upsert(&remote.Kite, value); err != nil {
		k.log.Error("storage add '%s' error: %s", remote.Kite, err)
		return nil, errors.New("internal error - register")
	}

	// if there is already just reset it
	k.heartbeatsMu.Lock()
	defer k.heartbeatsMu.Unlock()

	updateTimer, ok := k.heartbeats[remote.Kite.ID]
	if ok {
		// there is already a previous registration, use it
		k.log.Info("Kite was already register (via HTTP), use timer cache %s", remote.Kite)
		updateTimer.Reset(HeartbeatInterval + HeartbeatDelay)
		k.heartbeats[remote.Kite.ID] = updateTimer
	} else {
		// we create a new ticker which is going to update the key periodically in
		// the storage so it's always up to date. Instead of updating the key
		// periodically according to the HeartBeatInterval below, we are buffering
		// the write speed here with the UpdateInterval.
		stopped := make(chan struct{})
		updater := time.NewTicker(UpdateInterval)
		updaterFunc := func() {
			for {
				select {
				case <-updater.C:
					k.log.Info("Kite is active (via HTTP), updating the value %s", remote.Kite)
					err := k.storage.Update(&remote.Kite, value)
					if err != nil {
						k.log.Error("storage update '%s' error: %s", remote.Kite, err)
					}
				case <-stopped:
					k.log.Info("Kite is nonactive (via HTTP). Updater is closed %s", remote.Kite)
					return
				}
			}
		}
		go updaterFunc()

		// we are now creating a timer that is going to call the function which
		// stops the background updater if it's not resetted. The time is being
		// resetted on a separate HTTP endpoint "/heartbeat"
		k.heartbeats[remote.Kite.ID] = time.AfterFunc(HeartbeatInterval+HeartbeatDelay, func() {
			k.log.Info("Kite didn't sent any heartbeat (via HTTP). Stopping the updater %s",
				remote.Kite)
			// stop the updater so it doesn't update it in the background
			updater.Stop()

			k.heartbeatsMu.Lock()
			defer k.heartbeatsMu.Unlock()

			select {
			case <-stopped:
			default:
				close(stopped)
			}

			delete(k.heartbeats, remote.Kite.ID)
		})
	}

	k.log.Info("Kite registered (via HTTP): %s", remote.Kite)

	// send response back to the kite, also identify him with the new name
	return &protocol.RegisterResult{
		URL:               args.URL,
		HeartbeatInterval: int64(HeartbeatInterval / time.Second),
	}, nil
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	k.log.Info("Register request from: %s", r.Client.Kite)

	if r.Args.One().MustMap()["url"].MustString() == "" {
		return nil, errors.New("invalid url")
	}

	var args struct {
		URL string `json:"url"`
	}
	r.Args.One().MustUnmarshal(&args)
	if args.URL == "" {
		return nil, errors.New("empty url")
	}

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Auth.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Auth.Type)
	}

	kiteURL := args.URL
	remote := r.Client

	if err := validateKiteKey(&remote.Kite); err != nil {
		return nil, err
	}

	value := &kontrolprotocol.RegisterValue{
		URL: kiteURL,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Upsert(&remote.Kite, value); err != nil {
		k.log.Error("storage add '%s' error: %s", remote.Kite, err)
		return nil, errors.New("internal error - register")
	}

	every := onceevery.New(UpdateInterval)

	ping := make(chan struct{}, 1)
	closed := false

	updaterFunc := func() {
		for {
			select {
			case <-ping:
				k.log.Debug("Kite is active, got a ping %s", remote.Kite)
				every.Do(func() {
					k.log.Info("Kite is active, updating the value %s", remote.Kite)
					err := k.storage.Update(&remote.Kite, value)
					if err != nil {
						k.log.Error("storage update '%s' error: %s", remote.Kite, err)
					}
				})
			case <-time.After(HeartbeatInterval + HeartbeatDelay):
				k.log.Info("Kite didn't sent any heartbeat %s.", remote.Kite)
				every.Stop()
				closed = true
				return
			}
		}
	}
	go updaterFunc()

	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		dnode.Callback(func(args *dnode.Partial) {
			k.log.Debug("Kite send us an heartbeat. %s", remote.Kite)

			k.clientLocks.Get(remote.Kite.ID).Lock()
			defer k.clientLocks.Get(remote.Kite.ID).Unlock()

			select {
			case ping <- struct{}{}:
			default:
			}

			// seems we miss a heartbeat, so start it again!
			if closed {
				closed = false
				k.log.Warning("Updater was closed, but we are still getting heartbeats. Starting again %s",
					remote.Kite)

				// it might be removed because the ttl cleaner would come
				// before us, so try to add it again, the updater will than
				// continue to update it afterwards.
				k.storage.Upsert(&remote.Kite, value)
				go updaterFunc()
			}
		}),
	}

	// now trigger the remote kite so it sends us periodically an heartbeat
	remote.GoWithTimeout("kite.heartbeat", 4*time.Second, heartbeatArgs...)

	k.log.Info("Kite registered: %s", remote.Kite)

	remote.OnDisconnect(func() {
		k.log.Info("Kite disconnected: %s", remote.Kite)
		every.Stop()
	})

	// send response back to the kite, also identify him with the new name
	return &protocol.RegisterResult{URL: args.URL}, nil
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	// This type is here until inversion branch is merged.
	// Reason: We can't use the same struct for marshaling and unmarshaling.
	// TODO use the struct in protocol
	type GetKitesArgs struct {
		Query *protocol.KontrolQuery `json:"query"`
	}

	var args GetKitesArgs
	r.Args.One().MustUnmarshal(&args)

	query := args.Query

	// audience will go into the token as "aud" claim.
	audience := getAudience(query)

	// Generate token once here because we are using the same token for every
	// kite we return and generating many tokens is really slow.
	token, err := generateToken(audience, r.Username,
		k.Kite.Kite().Username, k.privateKey)
	if err != nil {
		return nil, err
	}

	// Get kites from the storage
	kites, err := k.storage.Get(query)
	if err != nil {
		return nil, err
	}

	// Attach tokens to kites
	kites.Attach(token)

	return &protocol.GetKitesResult{
		Kites: kites,
	}, nil
}

func (k *Kontrol) handleGetToken(r *kite.Request) (interface{}, error) {
	var query *protocol.KontrolQuery
	err := r.Args.One().Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query")
	}

	// check if it's exist
	kites, err := k.storage.Get(query)
	if err != nil {
		return nil, err
	}

	if len(kites) > 1 {
		return nil, errors.New("query matches more than one kite")
	}

	audience := getAudience(query)

	return generateToken(audience, r.Username, k.Kite.Kite().Username, k.privateKey)
}

func (k *Kontrol) handleMachine(r *kite.Request) (interface{}, error) {
	if k.MachineAuthenticate != nil {
		if err := k.MachineAuthenticate(r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	username := r.Args.One().MustString() // username should be send as an argument
	return k.registerUser(username)
}
