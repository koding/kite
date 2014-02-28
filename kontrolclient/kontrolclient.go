package kontrolclient

import (
	"container/list"
	"errors"
	"net/url"
	"sync"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

// Kontrol is a client for registering and querying Kontrol Kite.
type Kontrol struct {
	*kite.RemoteKite

	LocalKite *kite.Kite

	// used for synchronizing methods that needs to be called after
	// successful connection.
	ready chan bool

	// Watchers are saved here to re-watch on reconnect.
	watchers      *list.List
	watchersMutex sync.RWMutex
}

// NewKontrol returns a pointer to new Kontrol instance.
func New(k *kite.Kite) *Kontrol {
	if k.Config.KontrolURL == nil {
		panic("no kontrol URL given in config")
	}

	remoteKite := k.NewRemoteKite(k.Config.KontrolURL)
	remoteKite.Kite = protocol.Kite{Name: "kontrol"} // for logging purposes
	remoteKite.Authentication = &kite.Authentication{
		Type: "kiteKey",
		Key:  k.Config.KiteKey,
	}

	kontrol := &Kontrol{
		RemoteKite: remoteKite,
		LocalKite:  k,
		ready:      make(chan bool),
		watchers:   list.New(),
	}

	var once sync.Once

	kontrol.OnConnect(func() {
		k.Log.Info("Connected to Kontrol ")

		// signal all other methods that are listening on this channel, that we
		// are ready.
		once.Do(func() { close(kontrol.ready) })

		// Re-register existing watchers.
		kontrol.watchersMutex.RLock()
		for e := kontrol.watchers.Front(); e != nil; e = e.Next() {
			watcher := e.Value.(*Watcher)
			if err := watcher.rewatch(); err != nil {
				kontrol.Log.Error("Cannot rewatch query: %+v", watcher)
			}
		}
		kontrol.watchersMutex.RUnlock()
	})

	return kontrol
}

type registerResult struct {
	URL *url.URL
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() method.
func (k *Kontrol) Register(kiteURL *url.URL) (*registerResult, error) {
	<-k.ready

	args := protocol.RegsiterArgs{
		URL: kiteURL.String(),
	}

	response, err := k.RemoteKite.Tell("register", args)
	if err != nil {
		return nil, err
	}

	var rr protocol.RegisterResult
	err = response.Unmarshal(&rr)
	if err != nil {
		return nil, err
	}

	k.Log.Info("Registered to kontrol with URL: %s", rr.URL)

	parsed, err := url.Parse(rr.URL)
	if err != nil {
		k.Log.Error("Cannot parse registered URL: %s", err.Error())
	}

	return &registerResult{parsed}, nil
}

// WatchKites watches for Kites that matches the query. The onEvent functions
// is called for current kites and every new kite event.
func (k *Kontrol) WatchKites(query protocol.KontrolQuery, onEvent EventHandler) (*Watcher, error) {
	<-k.ready

	watcherID, err := k.watchKites(query, onEvent)
	if err != nil {
		return nil, err
	}

	return k.newWatcher(watcherID, &query, onEvent), nil
}

func (k *Kontrol) eventCallbackHandler(onEvent EventHandler) kite.Callback {
	return func(r *kite.Request) {
		var returnEvent *Event
		var returnError error

		args := r.Args.MustSliceOfLength(2)

		// Unmarshal event argument
		if args[0] != nil {
			var event = Event{localKite: k.LocalKite}
			err := args[0].Unmarshal(&event)
			if err != nil {
				k.Log.Error(err.Error())
				return
			}
			returnEvent = &event
		}

		// Unmarshal error argument
		if args[1] != nil {
			var kiteErr kite.Error
			err := args[1].Unmarshal(&kiteErr)
			if err != nil {
				k.Log.Error(err.Error())
				return
			}
			returnError = &kiteErr
		}

		onEvent(returnEvent, returnError)
	}
}

func (k *Kontrol) watchKites(query protocol.KontrolQuery, onEvent EventHandler) (watcherID string, err error) {
	remoteKites, watcherID, err := k.getKites(query, k.eventCallbackHandler(onEvent))
	if err != nil && err != ErrNoKitesAvailable {
		return "", err // return only when something really happened
	}

	// also put the current kites to the eventChan.
	for _, remoteKite := range remoteKites {
		event := Event{
			KiteEvent: protocol.KiteEvent{
				Action: protocol.Register,
				Kite:   remoteKite.Kite,
				Token:  remoteKite.Authentication.Key,
			},
			localKite: k.LocalKite,
		}

		onEvent(&event, nil)
	}

	return watcherID, nil
}

// GetKites returns the list of Kites matching the query. The returned list
// contains ready to connect RemoteKite instances. The caller must connect
// with RemoteKite.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kontrol) GetKites(query protocol.KontrolQuery) ([]*kite.RemoteKite, error) {
	remoteKites, _, err := k.getKites(query)
	if err != nil {
		return nil, err
	}

	if len(remoteKites) == 0 {
		return nil, ErrNoKitesAvailable
	}

	return remoteKites, nil
}

// used internally for GetKites() and WatchKites()
func (k *Kontrol) getKites(args ...interface{}) (kites []*kite.RemoteKite, watcherID string, err error) {
	<-k.ready

	response, err := k.RemoteKite.Tell("getKites", args...)
	if err != nil {
		return nil, "", err
	}

	var result = new(protocol.GetKitesResult)
	err = response.Unmarshal(&result)
	if err != nil {
		return nil, "", err
	}

	remoteKites := make([]*kite.RemoteKite, len(result.Kites))
	for i, currentKite := range result.Kites {
		_, err := jwt.Parse(currentKite.Token, k.LocalKite.RSAKey)
		if err != nil {
			return nil, result.WatcherID, err
		}

		// exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
		auth := &kite.Authentication{
			Type: "token",
			Key:  currentKite.Token,
			// validUntil: &exp,
		}

		parsed, err := url.Parse(currentKite.URL)
		if err != nil {
			k.Log.Error("invalid url came from kontrol", currentKite.URL)
		}

		remoteKites[i] = k.LocalKite.NewRemoteKiteString(currentKite.URL)
		remoteKites[i].Kite = currentKite.Kite
		remoteKites[i].URL = parsed
		remoteKites[i].Authentication = auth
	}

	return remoteKites, result.WatcherID, nil
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
