package kontrol

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/kontrol/onceevery"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

func (k *Kontrol) HandleRegister(r *kite.Request) (interface{}, error) {
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

	t, err := jwt.Parse(r.Auth.Key, kitekey.GetKontrolKey)
	if err != nil {
		return nil, err
	}

	publicKey, ok := t.Claims["kontrolKey"].(string)
	if !ok {
		return nil, errors.New("public key is not passed")
	}

	var keyPair *KeyPair
	var newKey bool

	// check if the key is valid and is stored in the key pair storage, if not
	// check if there is a new key we can use.
	keyPair, err = k.keyPair.GetKeyFromPublic(strings.TrimSpace(publicKey))
	if err != nil {
		newKey = true
		keyPair, err = k.pickKey(r)
		if err != nil {
			return nil, err // nothing to do here ..
		}
	}

	kiteURL := args.URL
	remote := r.Client

	if err := validateKiteKey(&remote.Kite); err != nil {
		return nil, err
	}

	value := &kontrolprotocol.RegisterValue{
		URL:   kiteURL,
		KeyID: keyPair.ID,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Upsert(&remote.Kite, value); err != nil {
		k.log.Error("storage add '%s' error: %s", remote.Kite, err)
		return nil, errors.New("internal error - register")
	}

	every := onceevery.New(UpdateInterval)

	ping := make(chan struct{}, 1)
	closed := int32(0)

	updaterFunc := func() {
		for {
			select {
			case <-k.closed:
				return
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
				atomic.StoreInt32(&closed, 1)
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
			if atomic.CompareAndSwapInt32(&closed, 1, 0) {
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
	resp := remote.GoWithTimeout("kite.heartbeat", 4*time.Second, heartbeatArgs...)

	go func() {
		if err := (<-resp).Err; err != nil {
			k.log.Error("failed requesting heartbeats from %q kite: %s", remote.Kite.Name, err)
		}
	}()

	k.log.Info("Kite registered: %s", remote.Kite)

	remote.OnDisconnect(func() {
		k.log.Info("Kite disconnected: %s", remote.Kite)
		every.Stop()
	})

	// send response back to the kite, also send the new public Key if it's exist
	p := &protocol.RegisterResult{URL: args.URL}
	if newKey {
		p.PublicKey = keyPair.Public
	}

	return p, nil
}

func (k *Kontrol) HandleGetKites(r *kite.Request) (interface{}, error) {
	var args protocol.GetKitesArgs
	if err := r.Args.One().Unmarshal(&args); err != nil {
		return nil, err
	}

	// Get kites from the storage
	kites, err := k.storage.Get(args.Query)
	if err != nil {
		return nil, err
	}

	for _, kite := range kites {
		// audience will go into the token as "aud" claim.
		audience := getAudience(args.Query)

		keyPair, err := k.keyPair.GetKeyFromID(kite.KeyID)
		if err != nil {
			return nil, err
		}

		// Generate token once here because we are using the same token for every
		// kite we return and generating many tokens is really slow.
		token, err := generateToken(audience, r.Username,
			k.Kite.Kite().Username, keyPair.Private)
		if err != nil {
			return nil, err
		}

		kite.Token = token
	}

	return &protocol.GetKitesResult{
		Kites: kites,
	}, nil
}

func (k *Kontrol) HandleGetToken(r *kite.Request) (interface{}, error) {
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

	if len(kites) == 0 {
		return nil, errors.New("no kites found")
	}

	kite := kites[0]
	audience := getAudience(query)

	keyPair, err := k.keyPair.GetKeyFromID(kite.KeyID)
	if err != nil {
		return nil, err
	}

	return generateToken(audience, r.Username, k.Kite.Kite().Username, keyPair.Private)
}

func (k *Kontrol) HandleMachine(r *kite.Request) (interface{}, error) {
	var args struct {
		AuthType string
	}

	err := r.Args.One().Unmarshal(&args)
	if err != nil {
		return nil, err
	}

	if k.MachineAuthenticate != nil {
		// an empty authType is ok, the implementer is responsible of it. It
		// can care of it or it can return an error
		if err := k.MachineAuthenticate(args.AuthType, r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	keyPair, err := k.pickKey(r)
	if err != nil {
		return nil, err
	}

	return k.registerUser(r.Client.Kite.Username, keyPair.Public, keyPair.Private)
}

func (k *Kontrol) HandleGetKey(r *kite.Request) (interface{}, error) {
	// Only accept requests with kiteKey because we need this info
	// for checking if the key is valid and needs to be regenerated
	if r.Auth.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Auth.Type)
	}

	t, err := jwt.Parse(r.Auth.Key, kitekey.GetKontrolKey)
	if err != nil {
		return nil, err
	}

	publicKey, ok := t.Claims["kontrolKey"].(string)
	if !ok {
		return nil, errors.New("public key is not passed")
	}

	err = k.keyPair.IsValid(publicKey)
	if err == nil {
		// everything is ok, just return the old one
		return publicKey, nil
	}

	keyPair, err := k.pickKey(r)
	if err != nil {
		return nil, err
	}

	return keyPair.Public, nil
}

func (k *Kontrol) pickKey(r *kite.Request) (*KeyPair, error) {
	if k.MachineKeyPicker != nil {
		keyPair, err := k.MachineKeyPicker(r)
		if err != nil {
			return nil, err
		}
		return keyPair, nil
	}

	if len(k.lastPublic) != 0 && len(k.lastPrivate) != 0 {
		return &KeyPair{
			Public:  k.lastPublic[len(k.lastPublic)-1],
			Private: k.lastPrivate[len(k.lastPrivate)-1],
			ID:      k.lastIDs[len(k.lastIDs)-1],
		}, nil
	}

	k.log.Error("neither machineKeyPicker nor public/private keys are available")
	return nil, errors.New("internal error - 1")
}
