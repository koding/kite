package kite

import (
	"errors"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

const (
	renewBefore   = 30 * time.Second
	retryInterval = 10 * time.Second
)

// TokenRenewer renews the token of a Client just before it expires.
type TokenRenewer struct {
	client           *Client
	localKite        *Kite
	validUntil       time.Time
	signalRenewToken chan struct{}
	disconnect       chan struct{}
}

func NewTokenRenewer(r *Client, k *Kite) (*TokenRenewer, error) {
	t := &TokenRenewer{
		client:           r,
		localKite:        k,
		signalRenewToken: make(chan struct{}, 1),
		disconnect:       make(chan struct{}),
	}
	return t, t.parse(r.Auth.Key)
}

// parse the token string and set
func (t *TokenRenewer) parse(tokenString string) error {
	token, err := jwt.Parse(tokenString, t.localKite.RSAKey)
	if err != nil {
		return fmt.Errorf("Cannot parse token: %s", err.Error())
	}

	exp, ok := token.Claims["exp"].(float64)
	if !ok {
		return errors.New("token: invalid exp claim")
	}

	t.validUntil = time.Unix(int64(exp), 0).UTC()
	return nil
}

// RenewWhenExpires renews the token before it expires.
func (t *TokenRenewer) RenewWhenExpires() {
	t.client.OnConnect(func() { go t.renewLoop() })
	t.client.OnDisconnect(func() { close(t.disconnect) })
}

func (t *TokenRenewer) renewLoop() {
	// renews token before it expires (sends the first signal to the goroutine below)
	go time.AfterFunc(t.renewDuration(), t.sendRenewTokenSignal)

	// renew token on signal util remote kite disconnects.
	for {
		select {
		case <-t.signalRenewToken:
			if err := t.renewToken(); err != nil {
				t.localKite.Log.Error("token renewer: %s Cannot renew token for Kite: %s I will retry in %d seconds...", err.Error(), t.client.ID, retryInterval/time.Second)
				// Need to sleep here litle bit because a signal is sent
				// when an expired token is detected on incoming request.
				// This sleep prevents the signal from coming too fast.
				time.Sleep(1 * time.Second)
				go time.AfterFunc(retryInterval, t.sendRenewTokenSignal)
			} else {
				go time.AfterFunc(t.renewDuration(), t.sendRenewTokenSignal)
			}
		case <-t.disconnect:
			return
		}
	}
}

// The duration from now to the time token needs to be renewed.
// Needs to be calculated after renewing the token.
func (t *TokenRenewer) renewDuration() time.Duration {
	return t.validUntil.Add(-renewBefore).Sub(time.Now().UTC())
}

func (t *TokenRenewer) sendRenewTokenSignal() {
	// Needs to be non-blocking because tokenRenewer may be stopped.
	select {
	case t.signalRenewToken <- struct{}{}:
	default:
	}
}

// renewToken gets a new token from a kontrolClient, parses it and sets it as the token.
func (t *TokenRenewer) renewToken() error {
	tokenString, err := t.localKite.GetToken(&t.client.Kite)
	if err != nil {
		return err
	}

	if err = t.parse(tokenString); err != nil {
		return err
	}

	t.client.Auth.Key = tokenString
	return nil
}
