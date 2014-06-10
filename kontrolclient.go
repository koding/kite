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

const (
	kontrolRetryDuration = 10 * time.Second
	proxyRetryDuration   = 10 * time.Second
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

// kontrolClient is a kite for registering and querying Kites from Kontrol.
type kontrolClient struct {
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

// SetupKontrolClient setups and prepares a the kontrol instance. It connects
// to kontrol and reconnects again if there is any disconnections. This method
// is called internally whenever a kontrol client specific action is taking.
// However if you wish to connect earlier you may call this method.
func (k *Kite) SetupKontrolClient() error {
	if k.kontrol.Client != nil {
		return nil // already prepared
	}

	if k.Config.KontrolURL == nil {
		return errors.New("no kontrol URL given in config")
	}

	client := k.NewClient(k.Config.KontrolURL.String())
	client.Kite = protocol.Kite{Name: "kontrol"} // for logging purposes
	client.Authentication = &Authentication{
		Type: "kiteKey",
		Key:  k.Config.KiteKey,
	}

	k.kontrol.Client = client
	k.kontrol.watchers = list.New()

	k.kontrol.OnConnect(func() {
		k.Log.Info("Connected to Kontrol ")

		// try to re-register on connect
		if k.kontrol.lastRegisteredURL != nil {
			select {
			case k.kontrol.registerChan <- k.kontrol.lastRegisteredURL:
			default:
			}
		}

		// signal all other methods that are listening on this channel, that we
		// are connected to kontrol.
		k.kontrol.onceConnected.Do(func() { close(k.kontrol.readyConnected) })

		// Re-register existing watchers.
		k.kontrol.watchersMutex.RLock()
		for e := k.kontrol.watchers.Front(); e != nil; e = e.Next() {
			watcher := e.Value.(*Watcher)
			if err := watcher.rewatch(); err != nil {
				k.Log.Error("Cannot rewatch query: %+v", watcher)
			}
		}
		k.kontrol.watchersMutex.RUnlock()
	})

	k.kontrol.OnDisconnect(func() {
		k.Log.Warning("Disconnected from Kontrol.")
	})

	// non blocking, is going to reconnect if the connection goes down.
	if _, err := k.kontrol.DialForever(); err != nil {
		return err
	}

	return nil
}

// WatchKites watches for Kites that matches the query. The onEvent functions
// is called for current kites and every nekite event.
func (k *Kite) WatchKites(query protocol.KontrolQuery, onEvent EventHandler) (*Watcher, error) {
	if err := k.SetupKontrolClient(); err != nil {
		return nil, err
	}

	<-k.kontrol.readyConnected

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
				URL:    client.URL,
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
	if err := k.SetupKontrolClient(); err != nil {
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
	<-k.kontrol.readyConnected

	response, err := k.kontrol.TellWithTimeout("getKites", 4*time.Second, args)
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

		clients[i] = k.NewClient(currentKite.URL)
		clients[i].Kite = currentKite.Kite
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
	if err := k.SetupKontrolClient(); err != nil {
		return "", err
	}

	<-k.kontrol.readyConnected

	result, err := k.kontrol.TellWithTimeout("getToken", 4*time.Second, kite)
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

	r := e.localKite.NewClient(e.URL)
	r.Kite = e.Kite
	r.Authentication = &Authentication{
		Type: "token",
		Key:  e.Token,
	}
	return r
}

// KontrolReadyNotify returns a channel that is closed when a successful
// registiration to kontrol is done.
func (k *Kite) KontrolReadyNotify() chan struct{} {
	return k.kontrol.readyRegistered
}

// signalReady is an internal method to notify that a sucessful registiration
// is done.
func (k *Kite) signalReady() {
	k.kontrol.onceRegistered.Do(func() { close(k.kontrol.readyRegistered) })
}

// RegisterForever is equilavent to Register(), but it tries to re-register if
// there is a disconnection. The returned error is for the first register
// attempt. It returns nil if ReadNotify() is ready and it's registered
// succesfull.
func (k *Kite) RegisterForever(kiteURL *url.URL) error {
	errs := make(chan error, 1)
	go func() {
		for u := range k.kontrol.registerChan {
			_, err := k.Register(u)
			if err == nil {
				k.kontrol.lastRegisteredURL = u
				k.signalReady()
				continue
			}

			select {
			case errs <- err:
			default:
			}

			k.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds",
				err, kontrolRetryDuration/time.Second)

			time.AfterFunc(kontrolRetryDuration, func() {
				select {
				case k.kontrol.registerChan <- u:
				default:
				}
			})
		}
	}()

	// don't block if there the given url is nil
	if kiteURL == nil {
		return nil
	}

	// initiate a registiration if a url is given
	k.kontrol.registerChan <- kiteURL

	select {
	case <-k.KontrolReadyNotify():
		return nil
	case err := <-errs:
		return err
	}
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() or WatchKites() method.  This method does not
// handle the reconnection case. If you want to keep registered to kontrol, use
// RegisterForever().
func (k *Kite) Register(kiteURL *url.URL) (*registerResult, error) {
	if err := k.SetupKontrolClient(); err != nil {
		return nil, err
	}

	<-k.kontrol.readyConnected

	args := protocol.RegisterArgs{
		URL: kiteURL.String(),
	}

	k.Log.Info("Registering to kontrol with URL: %s", kiteURL.String())

	response, err := k.kontrol.TellWithTimeout("register", 4*time.Second, args)
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

// RegisterToTunnel finds a tunnel proxy kite by asking kontrol then registers
// itselfs on proxy. On error, retries forever. On every successfull
// registration, it sends the proxied URL to the registerChan channel. There is
// no register URL needed because the Tunnel Proxy automatically gets the IP
// from tunneling. This is a blocking function.
func (k *Kite) RegisterToTunnel() {
	query := &protocol.KontrolQuery{
		Username:    k.Config.KontrolUser,
		Environment: k.Config.Environment,
		Name:        "tunnelproxy",
	}

	k.RegisterToProxy(nil, query)
}

// RegisterToProxy is just like RegisterForever but registers the given URL
// to kontrol over a kite-proxy. A Kiteproxy is a reverseproxy that can be used
// for SSL termination or handling hundreds of kites behind a single. This is a
// blocking function.
func (k *Kite) RegisterToProxy(registerURL *url.URL, query *protocol.KontrolQuery) {
	go k.RegisterForever(nil)

	for {
		var proxyKite *Client

		// The proxy kite to connect can be overriden with the
		// environmental variable "KITE_PROXY_URL". If it is not set
		// we will ask Kontrol for available Proxy kites.
		// As an authentication informain kiteKey method will be used,
		// so be careful when using this feature.
		kiteProxyURL := os.Getenv("KITE_PROXY_URL")
		if kiteProxyURL != "" {
			proxyKite = k.NewClient(kiteProxyURL)
			proxyKite.Authentication = &Authentication{
				Type: "kiteKey",
				Key:  k.Config.KiteKey,
			}
		} else {
			kites, err := k.GetKites(*query)
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

		proxyURL, err := k.registerToProxyKite(proxyKite, registerURL)
		if err != nil {
			time.Sleep(proxyRetryDuration)
			continue
		}

		k.kontrol.registerChan <- proxyURL

		// Block until disconnect from Proxy Kite.
		<-disconnect
	}
}

// registerToProxyKite dials the proxy kite and calls register method then
// returns the reverse-proxy URL.
func (k *Kite) registerToProxyKite(c *Client, kiteURL *url.URL) (*url.URL, error) {
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

	// do not panic if we call Tell method below
	if kiteURL == nil {
		kiteURL = &url.URL{}
	}

	// this could be tunnelproxy or reverseproxy. Tunnelproxy doesn't need an
	// URL however Reverseproxy needs one.
	result, err := c.TellWithTimeout("register", 4*time.Second, kiteURL.String())
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
