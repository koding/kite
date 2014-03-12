package registration

import (
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/protocol"
)

const (
	kontrolRetryDuration = 10 * time.Second
	proxyRetryDuration   = 10 * time.Second
)

type Registration struct {
	kontrolClient *kontrolclient.KontrolClient

	// To signal waiters when registration is successfull.
	ready     chan bool
	onceReady sync.Once
}

func New(kon *kontrolclient.KontrolClient) *Registration {
	return &Registration{
		kontrolClient: kon,
		ready:         make(chan bool),
	}
}

func (r *Registration) ReadyNotify() chan bool {
	return r.ready
}

func (r *Registration) signalReady() {
	r.onceReady.Do(func() { close(r.ready) })
}

// Register to Kontrol. This method is blocking.
func (r *Registration) RegisterToKontrol(kiteURL *url.URL) {
	urls := make(chan *url.URL, 1)
	urls <- kiteURL
	r.mainLoop(urls)
}

// Register to Proxy. This method is blocking.
func (r *Registration) RegisterToProxy() {
	r.keepRegisteredToProxyKite(nil)
}

// Register to Proxy, then Kontrol. This method is blocking.
func (r *Registration) RegisterToProxyAndKontrol() {
	urls := make(chan *url.URL, 1)

	go r.keepRegisteredToProxyKite(urls)
	r.mainLoop(urls)
}

func (r *Registration) mainLoop(urls chan *url.URL) {
	const (
		Connect = iota
		Disconnect
	)

	events := make(chan int)

	r.kontrolClient.OnConnect(func() { events <- Connect })
	r.kontrolClient.OnDisconnect(func() { events <- Disconnect })

	var lastRegisteredURL *url.URL

	for {
		select {
		case e := <-events:
			switch e {
			case Connect:
				r.kontrolClient.Log.Notice("Connected to Kontrol.")
				if lastRegisteredURL != nil {
					select {
					case urls <- lastRegisteredURL:
					default:
					}
				}
			case Disconnect:
				r.kontrolClient.Log.Warning("Disconnected from Kontrol.")
			}
		case u := <-urls:
			if _, err := r.kontrolClient.Register(u); err != nil {
				r.kontrolClient.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds", err, kontrolRetryDuration/time.Second)
				time.AfterFunc(kontrolRetryDuration, func() {
					select {
					case urls <- u:
					default:
					}
				})
			} else {
				lastRegisteredURL = u
				r.signalReady()
			}
		}
	}
}

// keepRegisteredToProxyKite finds a proxy kite by asking kontrol then registers
// itselfs on proxy. On error, retries forever. On every successfull
// registration, it sends the proxied URL to the urls channel. The caller must
// receive from this channel and should register to the kontrol with that URL.
// This function never returns.
func (r *Registration) keepRegisteredToProxyKite(urls chan<- *url.URL) {
	query := protocol.KontrolQuery{
		Username:    r.kontrolClient.LocalKite.Config.KontrolUser,
		Environment: r.kontrolClient.LocalKite.Config.Environment,
		Name:        "proxy",
	}

	for {
		kites, err := r.kontrolClient.GetKites(query)
		if err != nil {
			r.kontrolClient.Log.Error("Cannot get Proxy kites from Kontrol: %s", err.Error())
			time.Sleep(proxyRetryDuration)
			continue
		}

		// If more than one one Proxy Kite is available pick one randomly.
		// It does not matter which one we connect.
		proxy := kites[rand.Int()%len(kites)]

		// Notify us on disconnect
		disconnect := make(chan bool, 1)
		proxy.OnDisconnect(func() {
			select {
			case disconnect <- true:
			default:
			}
		})

		proxyURL, err := r.registerToProxyKite(proxy)
		if err != nil {
			time.Sleep(proxyRetryDuration)
			continue
		}

		if urls != nil {
			urls <- proxyURL
		} else {
			r.signalReady()
		}

		// Block until disconnect from Proxy Kite.
		<-disconnect
	}
}

// registerToProxyKite dials the proxy kite and calls register method then
// returns the reverse-proxy URL.
func (reg *Registration) registerToProxyKite(r *kite.Client) (*url.URL, error) {
	Log := reg.kontrolClient.Log

	err := r.Dial()
	if err != nil {
		Log.Error("Cannot connect to Proxy kite: %s", err.Error())
		return nil, err
	}

	// Disconnect from Proxy Kite if error happens while registering.
	defer func() {
		if err != nil {
			r.Close()
		}
	}()

	result, err := r.Tell("register")
	if err != nil {
		Log.Error("Proxy register error: %s", err.Error())
		return nil, err
	}

	proxyURL, err := result.String()
	if err != nil {
		Log.Error("Proxy register result error: %s", err.Error())
		return nil, err
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		Log.Error("Cannot parse Proxy URL: %s", err.Error())
		return nil, err
	}

	return parsed, nil
}
