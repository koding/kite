package kite

import (
	"container/list"
	"errors"
	"github.com/koding/kite/protocol"
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
	for url := range urls {
		k.URL = &protocol.KiteURL{*url}
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

	// Watchers are saved here to re-watch on reconnect.
	watchers      *list.List
	watchersMutex sync.RWMutex
}

// NewKontrol returns a pointer to new Kontrol instance.
func (k *Kite) NewKontrol(kontrolURL *url.URL) *Kontrol {
	// Only the address is required to connect Kontrol
	kite := protocol.Kite{
		Name: "kontrol", // for logging purposes
		URL:  &protocol.KiteURL{*kontrolURL},
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
		watchers:   list.New(),
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
	if ip == "0.0.0.0" {
		kite.URL.Host = net.JoinHostPort(rr.PublicIP, port)
	}

	k.Log.Info("Registered to Kontrol with URL: %s ID: %s", kite.URL.String(), kite.ID)

	// Save last registered URL to re-register on re-connect.
	k.lastRegisteredURL = &kite.URL.URL

	return nil
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

func (k *Kontrol) eventCallbackHandler(onEvent EventHandler) Callback {
	return func(r *Request) {
		var returnEvent *Event
		var returnError error

		args := r.Args.MustSliceOfLength(2)

		// Unmarshal event argument
		if args[0] != nil {
			var event = Event{localKite: k.localKite}
			err := args[0].Unmarshal(&event)
			if err != nil {
				k.Log.Error(err.Error())
				return
			}
			returnEvent = &event
		}

		// Unmarshal error argument
		if args[1] != nil {
			var kiteErr Error
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
			localKite: k.localKite,
		}

		onEvent(&event, nil)
	}

	return watcherID, nil
}

// GetKites returns the list of Kites matching the query. The returned list
// contains ready to connect RemoteKite instances. The caller must connect
// with RemoteKite.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kontrol) GetKites(query protocol.KontrolQuery) ([]*RemoteKite, error) {
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
func (k *Kontrol) getKites(args ...interface{}) (kites []*RemoteKite, watcherID string, err error) {
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

	remoteKites := make([]*RemoteKite, len(result.Kites))
	for i, kite := range result.Kites {
		token, err := jwt.Parse(kite.Token, k.localKite.getRSAKey)
		if err != nil {
			return nil, result.WatcherID, err
		}

		exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
		auth := Authentication{
			Type:       "token",
			Key:        kite.Token,
			validUntil: &exp,
		}

		remoteKites[i] = k.localKite.NewRemoteKite(kite.Kite, auth)
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

type Watcher struct {
	id       string
	query    *protocol.KontrolQuery
	handler  EventHandler
	kontrol  *Kontrol
	canceled bool
	mutex    sync.Mutex
	elem     *list.Element
}

type EventHandler func(*Event, error)

func (k *Kontrol) newWatcher(id string, query *protocol.KontrolQuery, handler EventHandler) *Watcher {
	watcher := &Watcher{
		id:      id,
		query:   query,
		handler: handler,
		kontrol: k,
	}

	// Add to the kontrol's watchers list.
	k.watchersMutex.Lock()
	watcher.elem = k.watchers.PushBack(watcher)
	k.watchersMutex.Unlock()

	return watcher
}

func (w *Watcher) Cancel() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.canceled {
		return nil
	}

	_, err := w.kontrol.Tell("cancelWatcher", w.id)
	if err == nil {
		w.canceled = true

		// Remove from kontrol's watcher list.
		w.kontrol.watchersMutex.Lock()
		w.kontrol.watchers.Remove(w.elem)
		w.kontrol.watchersMutex.Unlock()
	}

	return err
}

func (w *Watcher) rewatch() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	id, err := w.kontrol.watchKites(*w.query, w.handler)
	if err != nil {
		return err
	}
	w.id = id
	return nil
}
