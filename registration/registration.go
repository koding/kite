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
	registerKontrolRetryDuration = time.Minute
	proxyRetryDuration           = 10 * time.Second
)

type Registration struct {
	kontrol *kontrolclient.Kontrol

	// To signal waiters when registration is successfull.
	ready     chan bool
	onceReady sync.Once
}

func New(kon *kontrolclient.Kontrol) *Registration {
	return &Registration{
		kontrol: kon,
		ready:   make(chan bool),
	}
}

func (r *Registration) ReadyNotify() chan bool {
	return r.ready
}

func (r *Registration) signalReady() {
	r.onceReady.Do(func() { close(r.ready) })
}

// Register to Kontrol in background. If registration fails, it
func (r *Registration) RegisterToKontrol(kiteURL *url.URL) {
	for {
		_, err := r.kontrol.Register(kiteURL)
		if err != nil {
			// do not exit, because existing applications might import the kite package
			r.kontrol.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds", err, registerKontrolRetryDuration/time.Second)
			time.Sleep(registerKontrolRetryDuration)
			continue
		}
		r.signalReady()
		break
	}
}

// Register to Proxy in background.
func (r *Registration) RegisterToProxy() {
	r.keepRegisteredToProxyKite(nil)
}

// Register to Proxy, then Kontrol in background.
func (r *Registration) RegisterToProxyAndKontrol() {
	urls := make(chan *url.URL)

	go r.keepRegisteredToProxyKite(urls)

	for url := range urls {
		r.RegisterToKontrol(url)
	}
}

// keepRegisteredToProxyKite finds a proxy kite by asking kontrol then registers
// itselfs on proxy. On error, retries forever. On every successfull
// registration, it sends the proxied URL to the urls channel. The caller must
// receive from this channel and should register to the kontrol with that URL.
// This function never returns.
func (r *Registration) keepRegisteredToProxyKite(urls chan<- *url.URL) {
	query := protocol.KontrolQuery{
		Username:    r.kontrol.LocalKite.Config.KontrolUser,
		Environment: r.kontrol.LocalKite.Config.Environment,
		Name:        "proxy",
	}

	for {
		kites, err := r.kontrol.GetKites(query)
		if err != nil {
			r.kontrol.Log.Error("Cannot get Proxy kites from Kontrol: %s", err.Error())
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
func (reg *Registration) registerToProxyKite(r *kite.RemoteKite) (*url.URL, error) {
	Log := reg.kontrol.Log

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

	// reg.localKite.URL = &protocol.KiteURL{*parsed}

	return parsed, nil
}

// // func (r *Registerer) Start() {
// //   // Port is known here if "0" is used as port number
// //   host, _, _ := net.SplitHostPort(k.Kite.URL.Host)
// //   _, port, _ := net.SplitHostPort(addr.String())
// //   k.Kite.URL.Host = net.JoinHostPort(host, port)
// //   k.ServingURL.Host = k.Kite.URL.Host

// //   // We must connect to Kontrol after starting to listen on port
// //   if k.KontrolEnabled && k.Kontrol != nil {
// //       if err := k.Kontrol.DialForever(); err != nil {
// //           k.Log.Critical(err.Error())
// //       }

// //       if k.RegisterToKontrol {
// //           go k.keepRegisteredToKontrol(registerURLs)
// //       }
// //   }
// // }

// // Register to proxy and/or kontrol, then update the URL.
// func (k *Kite) Register(kiteURL *url.URL) {

// }

// remoteKite.OnConnect(func() {
//         // We need to re-register the last registered URL on re-connect.
//         if kontrol.lastRegisteredURL != nil {
//                 go kontrol.Register()
//         }
// })

// remoteKite.OnDisconnect(func() {
//         k.Log.Warning("Disconnected from Kontrol. I will retry in background...")
// })

// k.Log.Info("Registered to Kontrol with URL: %s ID: %s", kite.URL.String(), kite.ID)

// // Save last registered URL to re-register on re-connect.
// k.lastRegisteredURL = &kite.URL.URL
