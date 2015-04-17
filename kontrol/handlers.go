package kontrol

import (
	"errors"
	"fmt"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kontrol/onceevery"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

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
					k.log.Debug("Kite is active, updating the value %s", remote.Kite)
					err := k.storage.Update(&remote.Kite, value)
					if err != nil {
						k.log.Error("storage update '%s' error: %s", remote.Kite, err)
					}
				})
			case <-time.After(HeartbeatInterval + HeartbeatDelay):
				k.log.Debug("Kite didn't sent any heartbeat %s.", remote.Kite)
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
	var args struct {
		Username string
		AuthType string
	}

	if err := r.Args.One().Unmarshal(&args); err != nil {
		return nil, err
	}

	if args.Username == "" {
		return nil, errors.New("usename is empty")
	}

	if k.MachineAuthenticate != nil {
		// an empty authType is ok, the implementer is responsible of it. It
		// can care of it or it can return an error
		if err := k.MachineAuthenticate(args.AuthType, r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	return k.registerUser(args.Username)
}
