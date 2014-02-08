package kite

import (
	"errors"
	"kite/protocol"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

const registerKontrolRetryDuration = time.Minute

func (k *Kite) keepRegisteredToKontrol(urls chan *url.URL) {
	for k.URL.URL = range urls {
		for {
			err := k.Kontrol.Register()
			if err != nil {
				// do not exit, because existing applications might import the kite package
				k.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds", err, registerKontrolRetryDuration/time.Second)
				time.Sleep(registerKontrolRetryDuration)
			}

			// Registered to Kontrol.
			break
		}
	}
}

// Kontrol is a client for registering and querying Kontrol Kite.
type Kontrol struct {
	*RemoteKite

	// used for synchronizing methods that needs to be called after
	// successful connection.
	ready chan bool

	// Saved in order to re-register on re-connect.
	lastRegisteredURL *url.URL
}

// NewKontrol returns a pointer to new Kontrol instance.
func (k *Kite) NewKontrol(kontrolURL *url.URL) *Kontrol {
	// Only the address is required to connect Kontrol
	kite := protocol.Kite{
		Name: "kontrol", // for logging purposes
		URL:  protocol.KiteURL{kontrolURL},
	}

	auth := Authentication{
		Type: "kiteKey",
		Key:  k.kiteKey.Raw,
	}

	remoteKite := k.NewRemoteKite(kite, auth)
	remoteKite.client.Reconnect = true

	kontrol := &Kontrol{
		RemoteKite: remoteKite,
		ready:      make(chan bool),
	}

	var once sync.Once

	remoteKite.OnConnect(func() {
		k.Log.Info("Connected to Kontrol ")

		// We need to re-register the last registered URL on re-connect.
		if kontrol.lastRegisteredURL != nil {
			go kontrol.Register()
		}

		// signal all other methods that are listening on this channel, that we
		// are ready.
		once.Do(func() { close(kontrol.ready) })
	})

	remoteKite.OnDisconnect(func() {
		k.Log.Warning("Disconnected from Kontrol. I will retry in background...")
	})

	return kontrol
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() method.
func (k *Kontrol) Register() error {
	<-k.ready

	response, err := k.RemoteKite.Tell("register")
	if err != nil {
		return err
	}

	var rr protocol.RegisterResult
	err = response.Unmarshal(&rr)
	if err != nil {
		return err
	}

	kite := &k.localKite.Kite // shortcut

	// Set the correct PublicIP if left empty in options.
	ip, port, _ := net.SplitHostPort(kite.URL.Host)
	if ip == "" {
		kite.URL.Host = net.JoinHostPort(rr.PublicIP, port)
	}

	k.Log.Info("Registered to Kontrol with URL: %s ID: %s", kite.URL.String(), kite.ID)

	// Save last registered URL to re-register on re-connect.
	k.lastRegisteredURL = kite.URL.URL

	return nil
}

// WatchKites watches for Kites that matches the query. The onEvent functions
// is called for current kites and every new kite event.
func (k *Kontrol) WatchKites(query protocol.KontrolQuery, onEvent func(*Event)) error {
	<-k.ready

	queueEvents := func(r *Request) {
		event := Event{localKite: k.localKite}
		err := r.Args.MustSliceOfLength(1)[0].Unmarshal(&event)
		if err != nil {
			k.Log.Error(err.Error())
			return
		}

		onEvent(&event)
	}

	args := []interface{}{query, Callback(queueEvents)}
	remoteKites, err := k.getKites(args...)
	if err != nil && err != ErrNoKitesAvailable {
		return err // return only when something really happened
	}

	// also put the current kites to the eventChan.
	for _, remoteKite := range remoteKites {
		event := Event{
			KiteEvent: protocol.KiteEvent{
				Action: protocol.Register,
				Kite:   remoteKite.Kite,
				Token:  remoteKite.Authentication.Key,
			},
			localKite: k.localKite,
		}

		onEvent(&event)
	}

	return nil
}

// GetKites returns the list of Kites matching the query. The returned list
// contains ready to connect RemoteKite instances. The caller must connect
// with RemoteKite.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kontrol) GetKites(query protocol.KontrolQuery) ([]*RemoteKite, error) {
	return k.getKites(query)
}

// used internally for GetKites() and WatchKites()
func (k *Kontrol) getKites(args ...interface{}) ([]*RemoteKite, error) {
	<-k.ready

	response, err := k.RemoteKite.Tell("getKites", args...)
	if err != nil {
		return nil, err
	}

	var kites []protocol.KiteWithToken
	err = response.Unmarshal(&kites)
	if err != nil {
		return nil, err
	}

	if len(kites) == 0 {
		return nil, ErrNoKitesAvailable
	}

	remoteKites := make([]*RemoteKite, len(kites))
	for i, kite := range kites {
		token, err := jwt.Parse(kite.Token, k.localKite.getRSAKey)
		if err != nil {
			return nil, err
		}

		exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
		auth := Authentication{
			Type:       "token",
			Key:        kite.Token,
			validUntil: &exp,
		}

		remoteKites[i] = k.localKite.NewRemoteKite(kite.Kite, auth)
	}

	return remoteKites, nil
}

// GetToken is used to get a new token for a single Kite.
func (k *Kontrol) GetToken(kite *protocol.Kite) (string, error) {
	<-k.ready

	result, err := k.RemoteKite.Tell("getToken", kite)
	if err != nil {
		return "", err
	}

	var tkn string
	err = result.Unmarshal(&tkn)
	if err != nil {
		return "", err
	}

	return tkn, nil
}
