package kontrol

import (
	"errors"
	"fmt"
	"net/url"
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

	// Only accept requests with kiteKey because we need this info
	// for generating tokens for this kite.
	if r.Auth.Type != "kiteKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Auth.Type)
	}

	var args struct {
		URL string `json:"url"`
	}

	if err := r.Args.One().Unmarshal(&args); err != nil {
		return nil, err
	}

	if args.URL == "" {
		return nil, errors.New("empty url")
	}

	if _, err := url.Parse(args.URL); err != nil {
		return nil, fmt.Errorf("invalid register URL: %s", err)
	}

	res := &protocol.RegisterResult{
		URL: args.URL,
	}

	ex := &kitekey.Extractor{
		Claims: &kitekey.KiteClaims{},
	}

	t, err := jwt.ParseWithClaims(r.Auth.Key, ex.Claims, ex.Extract)
	if err != nil {
		return nil, err
	}

	var keyPair *KeyPair
	var origKey = ex.Claims.KontrolKey

	// check if the key is valid and is stored in the key pair storage, if not
	// check if there is a new key we can use.
	keyPair, res.KiteKey, err = k.getOrUpdateKeyPub(ex.Claims.KontrolKey, t, r)
	if err != nil {
		return nil, err
	}

	if origKey != keyPair.Public {
		// NOTE(rjeczalik): updates public key for old kites, new kites
		// expect kite key to be updated
		res.PublicKey = keyPair.Public
	}

	if err := validateKiteKey(&r.Client.Kite); err != nil {
		return nil, err
	}

	value := &kontrolprotocol.RegisterValue{
		URL:   args.URL,
		KeyID: keyPair.ID,
	}

	// Register first by adding the value to the storage. Return if there is
	// any error.
	if err := k.storage.Upsert(&r.Client.Kite, value); err != nil {
		k.log.Error("storage add '%s' error: %s", &r.Client.Kite, err)
		return nil, errors.New("internal error - register")
	}

	every := onceevery.New(UpdateInterval)

	ping := make(chan struct{}, 1)
	closed := int32(0)

	kiteCopy := r.Client.Kite

	updaterFunc := func() {
		for {
			select {
			case <-k.closed:
				return
			case <-ping:
				k.log.Debug("Kite is active, got a ping %s", &kiteCopy)
				every.Do(func() {
					k.log.Debug("Kite is active, updating the value %s", &kiteCopy)
					err := k.storage.Update(&kiteCopy, value)
					if err != nil {
						k.log.Error("storage update '%s' error: %s", &kiteCopy, err)
					}
				})
			case <-time.After(HeartbeatInterval + HeartbeatDelay):
				k.log.Debug("Kite didn't sent any heartbeat %s.", &kiteCopy)
				atomic.StoreInt32(&closed, 1)
				return
			}
		}
	}

	go updaterFunc()

	heartbeatArgs := []interface{}{
		HeartbeatInterval / time.Second,
		dnode.Callback(func(args *dnode.Partial) {
			k.log.Debug("Kite send us an heartbeat. %s", &kiteCopy)

			k.clientLocks.Get(kiteCopy.ID).Lock()
			defer k.clientLocks.Get(kiteCopy.ID).Unlock()

			select {
			case ping <- struct{}{}:
			default:
			}

			// seems we miss a heartbeat, so start it again!
			if atomic.CompareAndSwapInt32(&closed, 1, 0) {
				k.log.Warning("Updater was closed, but we are still getting heartbeats. Starting again %s", &kiteCopy)

				// it might be removed because the ttl cleaner would come
				// before us, so try to add it again, the updater will than
				// continue to update it afterwards.
				k.storage.Upsert(&kiteCopy, value)
				go updaterFunc()
			}
		}),
	}

	// now trigger the remote kite so it sends us periodically an heartbeat
	resp := r.Client.GoWithTimeout("kite.heartbeat", 4*time.Second, heartbeatArgs...)

	go func() {
		if err := (<-resp).Err; err != nil {
			k.log.Error("failed requesting heartbeats from %q kite: %s", kiteCopy.Name, err)
		}
	}()

	k.log.Info("Kite registered: %s", &r.Client.Kite)

	clientKite := r.Client.Kite.String()

	r.Client.OnDisconnect(func() {
		k.log.Info("Kite disconnected: %s", clientKite)
	})

	return res, nil
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
		keyPair, err := k.getOrUpdateKeyID(kite.KeyID, r)
		if err != nil {
			return nil, err
		}

		tok := &token{
			audience: getAudience(args.Query),
			username: r.Username,
			issuer:   k.Kite.Kite().Username,
			keyPair:  keyPair,
		}

		// Generate token once here because we are using the same token for every
		// kite we return and generating many tokens is really slow.
		token, err := k.generateToken(tok)
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
	var args protocol.GetTokenArgs

	if err := r.Args.One().Unmarshal(&args); err != nil {
		return nil, fmt.Errorf("invalid query: %s", err)
	}

	// check if it's exist
	kites, err := k.storage.Get(&args.KontrolQuery)
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

	keyPair, err := k.getOrUpdateKeyID(kite.KeyID, r)
	if err != nil {
		return nil, err
	}

	return k.generateToken(&token{
		audience: getAudience(&args.KontrolQuery),
		username: r.Username,
		issuer:   k.Kite.Kite().Username,
		keyPair:  keyPair,
		force:    args.Force,
	})
}

func (k *Kontrol) HandleMachine(r *kite.Request) (interface{}, error) {
	var args struct {
		AuthType string
	}

	err := r.Args.One().Unmarshal(&args)
	if err != nil {
		return nil, err
	}

	var keyPair *KeyPair

	if k.MachineAuthenticate != nil {
		// an empty authType is ok, the implementer is responsible of it. It
		// can care of it or it can return an error
		if err := k.MachineAuthenticate(args.AuthType, r); err != nil {
			k.Kite.Log.Error("machine authentication error: %s", err)

			return nil, fmt.Errorf("cannot authenticate user: %s", err)
		}

		keyPair, err = k.KeyPair()
	} else {
		keyPair, err = k.pickKey(r)
	}

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

	ex := &kitekey.Extractor{
		Claims: &kitekey.KiteClaims{},
	}

	if _, err := jwt.ParseWithClaims(r.Auth.Key, ex.Claims, ex.Extract); err != nil {
		return nil, err
	}

	if ex.Claims.KontrolKey == "" {
		return nil, errors.New("public key is not passed")
	}

	switch k.keyPair.IsValid(ex.Claims.KontrolKey) {
	case nil:
		// everything is ok, just return the old one
		return ex.Claims.KontrolKey, nil
	case ErrKeyDeleted:
		// client is using old key, update to current
		if kp, err := k.KeyPair(); err == nil {
			return kp.Public, nil
		}
	}

	keyPair, err := k.pickKey(r)
	if err != nil {
		return nil, err
	}

	return keyPair.Public, nil
}

func (k *Kontrol) HandleVerify(r *kite.Request) (interface{}, error) {
	return nil, nil
}

func (k *Kontrol) pickKey(r *kite.Request) (*KeyPair, error) {
	if k.MachineKeyPicker != nil {
		keyPair, err := k.MachineKeyPicker(r)
		if err != nil {
			return nil, err
		}

		return keyPair, nil
	}

	return nil, errors.New("no valid authentication key found")
}

func (k *Kontrol) updateKey(t *jwt.Token) (*KeyPair, string) {
	kp, err := k.KeyPair()
	if err != nil {
		k.log.Error("key update error for %q: %s", t.Claims.(*kitekey.KiteClaims).Subject, err)

		return nil, ""
	}

	if kiteKey := k.updateKeyWithKeyPair(t, kp); kiteKey != "" {
		return kp, kiteKey
	}

	return nil, ""
}

func (k *Kontrol) updateKeyWithKeyPair(t *jwt.Token, keyPair *KeyPair) string {
	claims := t.Claims.(*kitekey.KiteClaims)

	if claims.KontrolKey != "" {
		claims.KontrolKey = keyPair.Public
	}

	rsaPrivate, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(keyPair.Private))
	if err != nil {
		k.log.Error("key update error for %q: %s", claims.Subject, err)

		return ""
	}

	kiteKey, err := t.SignedString(rsaPrivate)
	if err != nil {
		k.log.Error("key update error for %q: %s", claims.Subject, err)

		return ""
	}

	return kiteKey
}

func (k *Kontrol) getOrUpdateKeyPub(pub string, t *jwt.Token, r *kite.Request) (*KeyPair, string, error) {
	var kiteKey string

	kp, err := k.keyPair.GetKeyFromPublic(pub)
	if err == ErrKeyDeleted {
		kp, kiteKey = k.updateKey(t)
	}

	if kp == nil {
		kp, err = k.pickKey(r)
		if err != nil {
			return nil, "", err
		}

		kiteKey = k.updateKeyWithKeyPair(t, kp)
	}

	return kp, kiteKey, nil
}

func (k *Kontrol) getOrUpdateKeyID(id string, r *kite.Request) (*KeyPair, error) {
	kp, err := k.keyPair.GetKeyFromID(id)
	if err == ErrKeyDeleted {
		kp, err = k.KeyPair()
		if err != nil {
			k.log.Error("key get or update error %q: %s", r.Username, err)
		}
	}

	if kp == nil {
		kp, err = k.pickKey(r)
		if err != nil {
			return nil, err
		}
	}

	return kp, nil
}
