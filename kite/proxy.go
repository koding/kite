package kite

import (
	"koding/newkite/protocol"
	"math/rand"
	"net/url"
	"time"
)

const proxyRetryDuration = 10 * time.Second

var proxyQuery = protocol.KontrolQuery{
	// Username must be "devrim" when running tests.
	// TODO Do not forget to change to "koding" before running on production.
	Username:    "devrim",
	Environment: "production",
	Name:        "proxy",
}

func (k *Kite) keepRegisteredToProxyKite(urls chan *url.URL) {
	for {
		kites, err := k.Kontrol.GetKites(proxyQuery)
		if err != nil {
			k.Log.Error("Cannot get Proxy kites from Kontrol: %s", err.Error())
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

		proxyURL, err := registerToProxyKite(proxy)
		if err != nil {
			time.Sleep(proxyRetryDuration)
			continue
		}

		if k.KontrolEnabled && k.RegisterToKontrol {
			urls <- proxyURL
		}

		// Block until disconnect from Proxy Kite.
		<-disconnect
	}
}

func registerToProxyKite(r *RemoteKite) (*url.URL, error) {
	Log := r.localKite.Log

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
