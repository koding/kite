// kontrolclient implements a kite.Client for interacting with Kontrol kite.
package kite

import (
	"container/list"
	"errors"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
)

type kontrolEvent int

const (
	kontrolRetryDuration = 10 * time.Second
	proxyRetryDuration   = 10 * time.Second
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

// KontrolClient is a kite for registering and querying Kites from Kontrol.
type KontrolClient struct {
	*Client

	// used for synchronizing methods that needs to be called after
	// successful connection or/and registiration to kontrol.
	onceConnected   sync.Once
	onceRegistered  sync.Once
	readyConnected  chan struct{}
	readyRegistered chan struct{}

	// watchers are saved here to re-watch on reconnect.
	watchers      *list.List
	watchersMutex sync.RWMutex

	// events is used for reconnection logic
	events chan kontrolEvent

	// lastRegisteredURL stores the Kite url what was send/registered
	// succesfully to kontrol
	lastRegisteredURL *url.URL

	// registerChan registers the url's it receives from the channel to Kontrol
	registerChan chan *url.URL
}

// Event is the struct that is emitted from Kontrol.WatchKites method.
type Event struct {
	protocol.KiteEvent

	localKite *Kite
}

type registerResult struct {
	URL *url.URL
}

// setupKontrolClient setups and prepares a the kontrol instance. It connects
// to kontrol and reconnects if needed.
func (k *Kite) setupKontrolClient() error {
	if k.Kontrol.Client != nil {
		return nil // already prepared
	}

	if k.Config.KontrolURL == nil {
		return errors.New("no kontrol URL given in config")
	}

	client := k.NewClient(k.Config.KontrolURL)
	client.Kite = protocol.Kite{Name: "kontrol"} // for logging purposes
	client.Authentication = &Authentication{
		Type: "kiteKey",
		Key:  k.Config.KiteKey,
	}

	k.Kontrol.Client = client
	k.Kontrol.watchers = list.New()

	k.Kontrol.OnConnect(func() {
		k.Log.Info("Connected to Kontrol ")

		// try to re-register on connect
		if k.Kontrol.lastRegisteredURL != nil {
			select {
			case k.Kontrol.registerChan <- k.Kontrol.lastRegisteredURL:
			default:
			}
		}

		// signal all other methods that are listening on this channel, that we
		// are connected to kontrol.
		k.Kontrol.onceConnected.Do(func() { close(k.Kontrol.readyConnected) })

		// Re-register existing watchers.
		k.Kontrol.watchersMutex.RLock()
		for e := k.Kontrol.watchers.Front(); e != nil; e = e.Next() {
			watcher := e.Value.(*Watcher)
			if err := watcher.rewatch(); err != nil {
				k.Log.Error("Cannot rewatch query: %+v", watcher)
			}
		}
		k.Kontrol.watchersMutex.RUnlock()
	})

	k.Kontrol.OnDisconnect(func() {
		k.Log.Warning("Disconnected from Kontrol.")
	})

	// non blocking, is going to reconnect if the connection goes down.
	if _, err := k.Kontrol.DialForever(); err != nil {
		return err
	}

	return nil
}

// WatchKites watches for Kites that matches the query. The onEvent functions
// is called for current kites and every new kite event.
func (k *Kite) WatchKites(query protocol.KontrolQuery, onEvent EventHandler) (*Watcher, error) {
	if err := k.setupKontrolClient(); err != nil {
		return nil, err
	}

	<-k.Kontrol.readyConnected

	watcherID, err := k.watchKites(query, onEvent)
	if err != nil {
		return nil, err
	}

	return k.newWatcher(watcherID, &query, onEvent), nil
}

func (k *Kite) eventCallbackHandler(onEvent EventHandler) dnode.Function {
	return dnode.Callback(func(args *dnode.Partial) {
		var response struct {
			Result *Event `json:"result"`
			Error  *Error `json:"error"`
		}

		args.One().MustUnmarshal(&response)

		if response.Result != nil {
			response.Result.localKite = k
		}

		onEvent(response.Result, response.Error)
	})
}

func (k *Kite) watchKites(query protocol.KontrolQuery, onEvent EventHandler) (watcherID string, err error) {
	args := protocol.GetKitesArgs{
		Query:         query,
		WatchCallback: k.eventCallbackHandler(onEvent),
	}
	clients, watcherID, err := k.getKites(args)
	if err != nil && err != ErrNoKitesAvailable {
		return "", err // return only when something really happened
	}

	// also put the current kites to the eventChan.
	for _, client := range clients {
		event := Event{
			KiteEvent: protocol.KiteEvent{
				Action: protocol.Register,
				Kite:   client.Kite,
				URL:    client.WSConfig.Location.String(),
				Token:  client.Authentication.Key,
			},
			localKite: k,
		}

		onEvent(&event, nil)
	}

	return watcherID, nil
}

// GetKites returns the list of Kites matching the query. The returned list
// contains Ready to connect Client instances. The caller must connect
// with Client.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kite) GetKites(query protocol.KontrolQuery) ([]*Client, error) {
	if err := k.setupKontrolClient(); err != nil {
		return nil, err
	}

	clients, _, err := k.getKites(protocol.GetKitesArgs{Query: query})
	if err != nil {
		return nil, err
	}

	if len(clients) == 0 {
		return nil, ErrNoKitesAvailable
	}

	return clients, nil
}

// used internally for GetKites() and WatchKites()
func (k *Kite) getKites(args protocol.GetKitesArgs) (kites []*Client, watcherID string, err error) {
	<-k.Kontrol.readyConnected

	response, err := k.Kontrol.Tell("getKites", args)
	if err != nil {
		return nil, "", err
	}

	var result = new(protocol.GetKitesResult)
	err = response.Unmarshal(&result)
	if err != nil {
		return nil, "", err
	}

	clients := make([]*Client, len(result.Kites))
	for i, currentKite := range result.Kites {
		_, err := jwt.Parse(currentKite.Token, k.RSAKey)
		if err != nil {
			return nil, result.WatcherID, err
		}

		// exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
		auth := &Authentication{
			Type: "token",
			Key:  currentKite.Token,
		}

		parsed, err := url.Parse(currentKite.URL)
		if err != nil {
			k.Log.Error("invalid url came from kontrol", currentKite.URL)
		}

		clients[i] = k.NewClientString(currentKite.URL)
		clients[i].Kite = currentKite.Kite
		clients[i].WSConfig.Location = parsed
		clients[i].Authentication = auth
	}

	// Renew tokens
	for _, r := range clients {
		token, err := NewTokenRenewer(r, k)
		if err != nil {
			k.Log.Error("Error in token. Token will not be renewed when it expires: %s", err.Error())
			continue
		}
		token.RenewWhenExpires()
	}

	return clients, result.WatcherID, nil
}

// GetToken is used to get a new token for a single Kite.
func (k *Kite) GetToken(kite *protocol.Kite) (string, error) {
	if err := k.setupKontrolClient(); err != nil {
		return "", err
	}

	<-k.Kontrol.readyConnected

	result, err := k.Kontrol.Tell("getToken", kite)
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

// Create new Client from Register events. It panics if the action is not
// Register.
func (e *Event) Client() *Client {
	if e.Action != protocol.Register {
		panic("This method can only be called for 'Register' event.")
	}

	r := e.localKite.NewClientString(e.URL)
	r.Kite = e.Kite
	r.Authentication = &Authentication{
		Type: "token",
		Key:  e.Token,
	}
	return r
}

func (k *Kite) ReadyNotify() chan struct{} {
	return k.Kontrol.readyRegistered
}

func (k *Kite) signalReady() {
	k.Kontrol.onceRegistered.Do(func() { close(k.Kontrol.readyRegistered) })
}

// Register to Kontrol. This method is blocking.
func (k *Kite) RegisterToKontrol(kiteURL *url.URL) {
	k.Kontrol.registerChan <- kiteURL
	k.mainLoop()
}

func (k *Kite) mainLoop() {
	for u := range k.Kontrol.registerChan {
		_, err := k.register(u)
		if err == nil {
			k.Kontrol.lastRegisteredURL = u
			k.signalReady()
			continue
		}

		k.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds",
			err, kontrolRetryDuration/time.Second)

		time.AfterFunc(kontrolRetryDuration, func() {
			select {
			case k.Kontrol.registerChan <- u:
			default:
			}
		})

	}
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() method.
//
// This method does not handle the reconnection case. If you want to keep
// registered to kontrol, use kite/registration package.
func (k *Kite) register(kiteURL *url.URL) (*registerResult, error) {
	if err := k.setupKontrolClient(); err != nil {
		return nil, err
	}

	<-k.Kontrol.readyConnected

	args := protocol.RegisterArgs{
		URL: kiteURL.String(),
	}

	response, err := k.Kontrol.Tell("register", args)
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

// RegisterToProxy finds a proxy kite by asking kontrol then registers itselfs
// on proxy. On error, retries forever. On every successfull registration, it
// sends the proxied URL to the registerChan channel (onlt if registerKontrol
// is enabled).  If registerKontrol is false it returns the proxy url and
// doesn't register himself to kontrol.
func (k *Kite) RegisterToProxy(registerToKontrol bool) *url.URL {
	if registerToKontrol {
		go k.mainLoop()
	}

	query := protocol.KontrolQuery{
		Username:    k.Config.KontrolUser,
		Environment: k.Config.Environment,
		Name:        "proxy",
	}

	for {
		var proxyKite *Client

		// The proxy kite to connect can be overriden with the
		// environmental variable "KITE_PROXY_URL". If it is not set
		// we will ask Kontrol for available Proxy kites.
		// As an authentication informain kiteKey method will be used,
		// so be careful when using this feature.
		kiteProxyURL := os.Getenv("KITE_PROXY_URL")
		if kiteProxyURL != "" {
			proxyKite = k.NewClientString(kiteProxyURL)
			proxyKite.Authentication = &Authentication{
				Type: "kiteKey",
				Key:  k.Config.KiteKey,
			}
		} else {
			kites, err := k.GetKites(query)
			if err != nil {
				k.Log.Error("Cannot get Proxy kites from Kontrol: %s", err.Error())
				time.Sleep(proxyRetryDuration)
				continue
			}

			// If more than one one Proxy Kite is available pick one randomly.
			// It does not matter which one we connect.
			proxyKite = kites[rand.Int()%len(kites)]
		}

		// Notify us on disconnect
		disconnect := make(chan bool, 1)
		proxyKite.OnDisconnect(func() {
			select {
			case disconnect <- true:
			default:
			}
		})

		proxyURL, err := k.registerToProxyKite(proxyKite)
		if err != nil {
			time.Sleep(proxyRetryDuration)
			continue
		}

		if registerToKontrol {
			k.Kontrol.registerChan <- proxyURL
		} else {
			k.signalReady()
		}

		// Block until disconnect from Proxy Kite.
		<-disconnect
	}
}

// registerToProxyKite dials the proxy kite and calls register method then
// returns the reverse-proxy URL.
func (k *Kite) registerToProxyKite(c *Client) (*url.URL, error) {
	err := c.Dial()
	if err != nil {
		k.Log.Error("Cannot connect to Proxy kite: %s", err.Error())
		return nil, err
	}

	// Disconnect from Proxy Kite if error happens while registering.
	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	result, err := c.Tell("register")
	if err != nil {
		k.Log.Error("Proxy register error: %s", err.Error())
		return nil, err
	}

	proxyURL, err := result.String()
	if err != nil {
		k.Log.Error("Proxy register result error: %s", err.Error())
		return nil, err
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		k.Log.Error("Cannot parse Proxy URL: %s", err.Error())
		return nil, err
	}

	return parsed, nil
}
