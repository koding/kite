package kite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/koding/kite/protocol"
)

type kontrolFunc func(*Client) error

// the implementation of New() doesn't have any error to be returned yet it
// returns, so it's totally safe to neglect the error
var cookieJar, _ = cookiejar.New(nil)

var defaultClient = &http.Client{
	Timeout: time.Second * 10,
	// add this so we can make use of load balancer's sticky session features,
	// such as AWS ELB
	Jar: cookieJar,
}

// kontrolFunc setups and prepares a kontrol instance. It connects to
// kontrol and providers a way to call the given function in that connected
// kontrol environment. This method is called internally whenever a kontrol
// client specific action is taking (getKites, getToken, register). The main
// reason for having this is doing the call and close the connection
// immediately, so there will be no persistent connection.
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

	if err := client.Dial(); err != nil {
		return err
	}
	defer client.Close()

	return fn(client)
}

// RegisterHTTPForever is just like RegisterHTTP however it first tries to
// register forever until a response from kontrol is received. It's useful to
// use it during app initializations. After the registration a reconnect is
// automatically handled inside the RegisterHTTP method.
func (k *Kite) RegisterHTTPForever(kiteURL *url.URL) {
	// Create the httpBackoffRegister that RegisterHTTPForever will
	// use to backoff repeated register attempts.
	httpRegisterBackOff := backoff.NewExponentialBackOff()
	httpRegisterBackOff.InitialInterval = 30 * time.Second
	httpRegisterBackOff.MaxInterval = 5 * time.Minute
	httpRegisterBackOff.Multiplier = 1.7
	httpRegisterBackOff.MaxElapsedTime = 0

	register := func() error {
		_, err := k.RegisterHTTP(kiteURL)
		if err != nil {
			k.Log.Error("Cannot register to Kontrol: %s Will retry after %d seconds",
				err,
				httpRegisterBackOff.NextBackOff()/time.Second)
			return err
		}

		return nil
	}

	// this will retry register forever
	err := backoff.Retry(register, httpRegisterBackOff)
	if err != nil {
		k.Log.Error("BackOff stopped retrying with Error '%s'", err)
	}
}

func (k *Kite) getKontrolPath(path string) string {
	heartbeatURL := k.Config.KontrolURL + "/" + path
	if strings.HasSuffix(k.Config.KontrolURL, "/kite") {
		heartbeatURL = strings.TrimSuffix(k.Config.KontrolURL, "/kite") + "/" + path
	}

	return heartbeatURL
}

// RegisterHTTP registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() or WatchKites() method. It registers again if
// connection to kontrol is lost.
func (k *Kite) RegisterHTTP(kiteURL *url.URL) (*registerResult, error) {
	registerURL := k.getKontrolPath("register")

	args := protocol.RegisterArgs{
		URL:  kiteURL.String(),
		Kite: k.Kite(),
		Auth: &protocol.Auth{
			Type: "kiteKey",
			Key:  k.Config.KiteKey,
		},
	}

	data, err := json.Marshal(&args)
	if err != nil {
		return nil, err
	}

	resp, err := defaultClient.Post(registerURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rr protocol.RegisterResult
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, err
	}

	if rr.Error != "" {
		return nil, errors.New(rr.Error)
	}

	if rr.HeartbeatInterval == 0 {
		return nil, errors.New("heartbeat interval cannot be zero")
	}

	// we also received a new public key (means the old one was invalidated).
	// Use it now.
	if rr.PublicKey != "" {
		k.Config.KontrolKey = rr.PublicKey
	}

	parsed, err := url.Parse(rr.URL)
	if err != nil {
		k.Log.Error("Cannot parse registered URL: %s", err.Error())
	}

	heartbeat := time.Duration(rr.HeartbeatInterval) * time.Second

	k.Log.Info("Registered (via HTTP) with URL: '%s' and HeartBeat interval: '%s'",
		rr.URL, heartbeat)

	go k.sendHeartbeats(heartbeat, kiteURL)

	return &registerResult{parsed}, nil
}

func (k *Kite) sendHeartbeats(interval time.Duration, kiteURL *url.URL) {
	tick := time.NewTicker(interval)

	heartbeatURL := k.getKontrolPath("heartbeat")

	k.Log.Debug("Starting to send heartbeat to: %s", heartbeatURL)

	u, err := url.Parse(heartbeatURL)
	if err != nil {
		k.Log.Fatal("HeartbeatURL is malformed: %s", err)
	}

	q := u.Query()
	q.Set("id", k.Id)
	u.RawQuery = q.Encode()

	errRegisterAgain := errors.New("register again")

	heartbeatFunc := func() error {
		k.Log.Debug("Sending heartbeat to %s", u.String())

		resp, err := defaultClient.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// we are just receving small size strings such as "pong",
		// "registeragain" so it's totally normal to consume the whole response
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		k.Log.Debug("Heartbeat response received '%s'", strings.TrimSpace(string(body)))

		switch string(body) {
		case "pong":
			return nil
		case "registeragain":
			k.Log.Info("Disconnected from Kontrol, going to register again")
			go func() {
				k.RegisterHTTPForever(kiteURL)
				tick.Stop()
			}()

			return errRegisterAgain
		}

		return fmt.Errorf("malformed heartbeat response %v", strings.TrimSpace(string(body)))
	}

	for _ = range tick.C {
		err := heartbeatFunc()
		if err == errRegisterAgain {
			return // return so we don't run forever
		}

		if err != nil {
			k.Log.Error("couldn't sent hearbeat: %s", err)
		}
	}
}
