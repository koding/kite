package kite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/koding/kite/protocol"
)

type heartbeatReq struct {
	ping     func() error
	interval time.Duration
}

func newHeartbeatReq(r *Request) (*heartbeatReq, error) {
	if r.Args == nil {
		return nil, errors.New("empty heartbeat request")
	}

	args, err := r.Args.SliceOfLength(2)
	if err != nil {
		return nil, err
	}

	d, err := args[0].Float64()
	if err != nil {
		return nil, err
	}

	ping, err := args[1].Function()
	if err != nil {
		return nil, err
	}

	return &heartbeatReq{
		interval: time.Duration(d) * time.Second,
		ping: func() error {
			return ping.Call()
		},
	}, nil
}

func (k *Kite) processHeartbeats() {
	var (
		ping func() error
		t    = time.NewTicker(time.Second) // dummy initial value
	)

	t.Stop()

	for {
		select {
		case <-t.C:
			switch err := ping(); err {
			case nil:
			case errRegisterAgain:
				t.Stop()
			default:
				k.Log.Error("%s", err)
			}
		case <-k.closeC:
			t.Stop()
			return
		case req := <-k.heartbeatC:
			t.Stop()

			if req == nil {
				continue
			}

			t = time.NewTicker(req.interval)
			ping = req.ping
		}
	}
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
			Key:  k.KiteKey(),
		},
	}

	data, err := json.Marshal(&args)
	if err != nil {
		return nil, err
	}

	resp, err := k.Config.Client.Post(registerURL, "application/json", bytes.NewReader(data))
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

	parsed, err := url.Parse(rr.URL)
	if err != nil {
		k.Log.Error("Cannot parse registered URL: %s", err.Error())
	}

	heartbeat := time.Duration(rr.HeartbeatInterval) * time.Second

	k.Log.Info("Registered (via HTTP) with URL: '%s' and HeartBeat interval: '%s'",
		rr.URL, heartbeat)

	go k.sendHeartbeats(heartbeat, kiteURL)

	k.callOnRegisterHandlers(&rr)

	return &registerResult{parsed}, nil
}

var errRegisterAgain = errors.New("register again")

func (k *Kite) sendHeartbeats(interval time.Duration, kiteURL *url.URL) {
	heartbeatURL := k.getKontrolPath("heartbeat")

	k.Log.Debug("Starting to send heartbeat to: %s", heartbeatURL)

	u, err := url.Parse(heartbeatURL)
	if err != nil {
		k.Log.Fatal("HeartbeatURL is malformed: %s", err)
	}

	q := u.Query()
	q.Set("id", k.Id)
	u.RawQuery = q.Encode()

	heartbeatFunc := func() error {
		k.Log.Debug("Sending heartbeat to %s", u)

		resp, err := k.Config.Client.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		// we are just receiving small size strings such as "pong",
		// "registeragain" so we limit the reader to read just that
		p, err := ioutil.ReadAll(io.LimitReader(resp.Body, 16))
		if err != nil {
			return err
		}

		p = bytes.TrimSpace(p)

		k.Log.Debug("Heartbeat response received %q", p)

		switch string(p) {
		case "pong":
			return nil
		case "registeragain":
			k.Log.Info("Disconnected from Kontrol, going to register again")

			go func() {
				k.RegisterHTTPForever(kiteURL)
			}()

			return errRegisterAgain
		}

		return fmt.Errorf("malformed heartbeat response: %s", p)
	}

	k.heartbeatC <- &heartbeatReq{
		ping:     heartbeatFunc,
		interval: interval,
	}
}

// handleHeartbeat pings the callback with the given interval seconds.
func (k *Kite) handleHeartbeat(r *Request) (interface{}, error) {
	req, err := newHeartbeatReq(r)
	if err != nil {
		return nil, err
	}

	k.heartbeatC <- req

	return nil, req.ping()
}
