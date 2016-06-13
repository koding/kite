// kontrolclient implements a kite.Client for interacting with Kontrol kite.
package kite

import (
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
)

const (
	kontrolRetryDuration = 10 * time.Second
	proxyRetryDuration   = 10 * time.Second

	// kontrolConnectTimeout is the timeout for connecting to Kontrol in
	// TellKontrol-like methods.
	kontrolConnectTimeout = 10 * time.Second
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

// kontrolClient is a kite for registering and querying Kites from Kontrol.
type kontrolClient struct {
	*Client
	sync.Mutex // protects Client

	// used for synchronizing methods that needs to be called after
	// successful connection or/and registiration to kontrol.
	onceConnected   sync.Once
	onceRegistered  sync.Once
	readyConnected  chan struct{}
	readyRegistered chan struct{}

	// lastRegisteredURL stores the Kite url what was send/registered
	// succesfully to kontrol
	lastRegisteredURL *url.URL

	// registerChan registers the url's it receives from the channel to Kontrol
	registerChan chan *url.URL
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

	if k.Config.KontrolURL == "" {
		return errors.New("no kontrol URL given in config")
	}

	client := k.NewClient(k.Config.KontrolURL)
	client.Kite = protocol.Kite{Name: "kontrol"} // for logging purposes
	client.Auth = &Auth{
		Type: "kiteKey",
		Key:  k.KiteKey(),
	}

	k.kontrol.Lock()
	k.kontrol.Client = client
	k.kontrol.Unlock()

	k.kontrol.OnConnect(func() {
		k.Log.Info("Connected to Kontrol")
		k.Log.Debug("Connected to Kontrol with session %q", client.session.ID())

		// try to re-register on connect
		k.kontrol.Lock()
		if k.kontrol.lastRegisteredURL != nil {
			select {
			case k.kontrol.registerChan <- k.kontrol.lastRegisteredURL:
			default:
			}
		}
		k.kontrol.Unlock()

		// signal all other methods that are listening on this channel, that we
		// are connected to kontrol.
		k.kontrol.onceConnected.Do(func() { close(k.kontrol.readyConnected) })
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

// GetKites returns the list of Kites matching the query. The returned list
// contains Ready to connect Client instances. The caller must connect
// with Client.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kite) GetKites(query *protocol.KontrolQuery) ([]*Client, error) {
	if err := k.SetupKontrolClient(); err != nil {
		return nil, err
	}

	clients, err := k.getKites(protocol.GetKitesArgs{Query: query})
	if err != nil {
		return nil, err
	}

	if len(clients) == 0 {
		return nil, ErrNoKitesAvailable
	}

	return clients, nil
}

// used internally for GetKites() and WatchKites()
func (k *Kite) getKites(args protocol.GetKitesArgs) ([]*Client, error) {
	<-k.kontrol.readyConnected

	response, err := k.kontrol.TellWithTimeout("getKites", 4*time.Second, args)
	if err != nil {
		return nil, err
	}

	var result = new(protocol.GetKitesResult)
	err = response.Unmarshal(&result)
	if err != nil {
		return nil, err
	}

	clients := make([]*Client, len(result.Kites))
	for i, currentKite := range result.Kites {
		auth := &Auth{
			Type: "token",
			Key:  currentKite.Token,
		}

		clients[i] = k.NewClient(currentKite.URL)
		clients[i].Kite = currentKite.Kite
		clients[i].Auth = auth
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

	return clients, nil
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

// GetKey is used to get a new public key from kontrol if the current one is
// invalidated. The key is also replaced in memory and every request is going
// to use it. This means even if kite.key contains the old key, the kite itself
// uses the new one.
func (k *Kite) GetKey() (string, error) {
	if err := k.SetupKontrolClient(); err != nil {
		return "", err
	}

	<-k.kontrol.readyConnected

	result, err := k.kontrol.TellWithTimeout("getKey", 4*time.Second)
	if err != nil {
		return "", err
	}

	var key string
	err = result.Unmarshal(&key)
	if err != nil {
		return "", err
	}

	k.configMu.Lock()
	k.Config.KontrolKey = key
	k.configMu.Unlock()

	return key, nil
}

// NewKeyRenewer renews the internal key every given interval
func (k *Kite) NewKeyRenewer(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for _ = range ticker.C {
		_, err := k.GetKey()
		if err != nil {
			k.Log.Warning("Key renew failed: %s", err)
		}
	}
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
				k.kontrol.Lock()
				k.kontrol.lastRegisteredURL = u
				k.kontrol.Unlock()
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

	k.Log.Info("Registered to kontrol with URL: %s and Kite query: %s",
		rr.URL, k.Kite())

	parsed, err := url.Parse(rr.URL)
	if err != nil {
		k.Log.Error("Cannot parse registered URL: %s", err)
	}

	k.callOnRegisterHandlers(&rr)

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
			proxyKite.Auth = &Auth{
				Type: "kiteKey",
				Key:  k.KiteKey(),
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

// TellKontrolWithTimeout is a lower level function for communicating directly with
// kontrol. Like GetKites and GetToken, this automatically sets up and connects to
// kontrol as needed.
func (k *Kite) TellKontrolWithTimeout(method string, timeout time.Duration, args ...interface{}) (result *dnode.Partial, err error) {
	if err := k.SetupKontrolClient(); err != nil {
		return nil, err
	}

	// Wait for readyConnect, or timeout
	select {
	case <-time.After(kontrolConnectTimeout):
		return nil, &Error{
			Type: "timeout",
			Message: fmt.Sprintf(
				"Timed out registering to kontrol for %s method after %s",
				method, kontrolConnectTimeout,
			),
		}
	case <-k.kontrol.readyConnected:
	}

	return k.kontrol.TellWithTimeout(method, timeout, args...)
}
