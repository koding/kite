// kontrolclient implements a kite.Client for interacting with Kontrol kite.
package kite

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
)

// Returned from GetKites when query matches no kites.
var ErrNoKitesAvailable = errors.New("no kites availabile")

type registerResult struct {
	URL *url.URL
}

type kontrolFunc func(*Client) error

// kontrolFunc setups and prepares a the kontrol instance. It connects to
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

// GetToken is used to get a new token for a single Kite.
func (k *Kite) GetToken(kite *protocol.Kite) (string, error) {
	var response *dnode.Partial
	getTokenFunc := func(kontrol *Client) error {
		var err error
		response, err = kontrol.TellWithTimeout("getToken", 4*time.Second, kite)
		return err
	}

	if err := k.kontrolFunc(getTokenFunc); err != nil {
		return "", err
	}

	var tkn string
	if err := response.Unmarshal(&tkn); err != nil {
		return "", err
	}

	return tkn, nil
}

// GetKites returns the list of Kites matching the query. The returned list
// contains Ready to connect Client instances. The caller must connect
// with Client.Dial() before using each Kite. An error is returned when no
// kites are available.
func (k *Kite) GetKites(query *protocol.KontrolQuery) ([]*Client, error) {
	var response *dnode.Partial
	getKitesFunc := func(kontrol *Client) error {
		args := protocol.GetKitesArgs{
			Query: query,
		}

		var err error
		response, err = kontrol.TellWithTimeout("getKites", 4*time.Second, args)
		return err
	}

	if err := k.kontrolFunc(getKitesFunc); err != nil {
		return nil, err
	}

	var result = new(protocol.GetKitesResult)

	if err := response.Unmarshal(&result); err != nil {
		return nil, err
	}

	if len(result.Kites) == 0 {
		return nil, ErrNoKitesAvailable
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

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() or WatchKites() method. It registers again if
// connection to kontrol is lost.
func (k *Kite) Register(kiteURL *url.URL) (*registerResult, error) {
	var response *dnode.Partial

	registerFunc := func(kontrol *Client) error {
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

	go k.sendHeartbeats(
		time.Duration(rr.HeartbeatInterval)*time.Second,
		kiteURL,
	)

	return &registerResult{parsed}, nil
}

func (k *Kite) sendHeartbeats(interval time.Duration, kiteURL *url.URL) {
	tick := time.NewTicker(interval)

	var heartbeatURL string
	if strings.HasSuffix(k.Config.KontrolURL, "/kite") {
		heartbeatURL = strings.TrimSuffix(k.Config.KontrolURL, "/kite") + "/heartbeat"
	} else {
		heartbeatURL = k.Config.KontrolURL + "/heartbeat"
	}

	u, err := url.Parse(heartbeatURL)
	if err != nil {
		k.Log.Fatal("HeartbeatURL is malformed: %s", err)
	}

	q := u.Query()
	q.Set("id", k.Id)
	u.RawQuery = q.Encode()

	heartbeatFunc := func() error {
		k.Log.Debug("Sending heartbeat to %s", u.String())

		resp, err := http.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// we are just receving the strings such as "pong", "registeragain" so
		// it's totally normal to consume the whole response
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		k.Log.Debug("Heartbeat response %s", string(body))

		switch string(body) {
		case "pong":
			return nil
		case "registeragain":
			tick.Stop()
			k.Register(kiteURL)
			return nil
		}

		return fmt.Errorf("malformed heartbeat response %v", string(body))
	}

	for _ = range tick.C {
		if err := heartbeatFunc(); err != nil {
			k.Log.Error("couldn't sent hearbeat: %s", err)
		}
	}
}
