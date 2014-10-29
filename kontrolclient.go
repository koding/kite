// kontrolclient implements a kite.Client for interacting with Kontrol kite.
package kite

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
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

type kontrolFunc func(*kontrolClient) error

func (k *Kite) kontrolFunc(fn kontrolFunc) error {
	if k.Config.KontrolURL == "" {
		return errors.New("no kontrol URL given in config")
	}

	client := k.NewClient(k.Config.KontrolURL)

	client.Kite = protocol.Kite{Name: "kontrol"} // for logging purposes
	client.Auth = &Auth{
		Type: "kiteKey",
		Key:  k.Config.KiteKey,
	}

	kontrol := &kontrolClient{}
	kontrol.Lock()
	kontrol.Client = client
	kontrol.Unlock()

	if err := kontrol.Dial(); err != nil {
		return err
	}
	defer kontrol.Close()

	return fn(kontrol)
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() or WatchKites() method.  This method does not
// handle the reconnection case. If you want to keep registered to kontrol, use
// RegisterForever().
func (k *Kite) Register(kiteURL *url.URL) (*registerResult, error) {
	var response *dnode.Partial

	registerFunc := func(kontrol *kontrolClient) error {
		args := protocol.RegisterArgs{
			URL: kiteURL.String(),
		}

		k.Log.Info("Registering to kontrol with URL: %s", kiteURL.String())
		var err error
		response, err = kontrol.TellWithTimeout("register", 4*time.Second, args)
		return err
	}

	if err := k.kontrolFunc(registerFunc); err != nil {
		return nil, err
	}

	var rr protocol.RegisterResult
	if err := response.Unmarshal(&rr); err != nil {
		return nil, err
	}

	k.Log.Info("Registered to kontrol with URL: %s and Kite query: %s",
		rr.URL, k.Kite())

	parsed, err := url.Parse(rr.URL)
	if err != nil {
		k.Log.Error("Cannot parse registered URL: %s", err.Error())
	}

	go k.sendHeartbeats(time.Duration(rr.HeartbeatInterval) * time.Second)

	return &registerResult{parsed}, nil
}

func (k *Kite) sendHeartbeats(interval time.Duration) {
	tick := time.Tick(interval)

	var heartbeatURL string
	if strings.HasSuffix(k.Config.KontrolURL, "/kite") {
		heartbeatURL = strings.TrimSuffix(k.Config.KontrolURL, "/kite") + "/heartbeat"
	} else {
		heartbeatURL = k.Config.KontrolURL + "/heartbeat"
	}

	for _ = range tick {
		if err := k.heartbeat(heartbeatURL); err != nil {
			k.Log.Error("couldn't sent hearbeat: %s", err)
		}
	}
}

func (k *Kite) heartbeat(url string) error {
	k.Log.Info("Sending heartbeat to %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// we are just receving the string "pong" so it's totally normal to consume
	// the whole response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if string(body) == "pong" {
		return nil
	}

	return fmt.Errorf("malformed heartbeat response %v", string(body))
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
		Key:  k.Config.KiteKey,
	}

	k.kontrol.Lock()
	k.kontrol.Client = client
	k.kontrol.disconnect = make(chan struct{})
	k.kontrol.Unlock()

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
	})

	k.kontrol.OnDisconnect(func() {
		k.Log.Warning("Disconnected from Kontrol.")
		close(k.kontrol.disconnect)
	})

	// non blocking, is going to reconnect if the connection goes down.
	if err := k.kontrol.Dial(); err != nil {
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
		_, err := jwt.Parse(currentKite.Token, k.RSAKey)
		if err != nil {
			return nil, err
		}

		// exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
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
