package kite

import (
	"fmt"
	"strings"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
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
	once             sync.Once // for c.installHandlers
	renewLoopWG      sync.WaitGroup
}

func NewTokenRenewer(r *Client, k *Kite) (*TokenRenewer, error) {
	t := &TokenRenewer{
		client:           r,
		localKite:        k,
		signalRenewToken: make(chan struct{}),
		disconnect:       make(chan struct{}),
	}
	return t, t.parse(r.Auth.Key)
}

// parse the token string and set
func (t *TokenRenewer) parse(tokenString string) error {
	claims := &kitekey.KiteClaims{}

	_, err := jwt.ParseWithClaims(tokenString, claims, t.localKite.RSAKey)
	if err != nil {
		valErr, ok := err.(*jwt.ValidationError)
		if !ok {
			return err
		}

		// do noy return for ValidationErrorSignatureValid. This is because we
		// might asked for a kite who's public Key is different what we have.
		// We still should be able to send them requests.
		if (valErr.Errors & jwt.ValidationErrorSignatureInvalid) == 0 {
			return fmt.Errorf("Cannot parse token: %s", err)
		}
	}

	t.validUntil = time.Unix(claims.ExpiresAt, 0).UTC()
	return nil
}

// RenewWhenExpires renews the token before it expires.
func (t *TokenRenewer) RenewWhenExpires() {
	t.once.Do(t.installHandlers)
}

func (t *TokenRenewer) installHandlers() {
	t.client.OnConnect(t.startRenewLoop)
	t.client.OnTokenExpire(t.sendRenewTokenSignal)
	t.client.OnDisconnect(t.sendDisconnectSignal)
}

func (t *TokenRenewer) renewLoop() {
	t.renewLoopWG.Add(1)
	defer t.renewLoopWG.Done()

	// renews token before it expires (sends the first signal to the goroutine below)
	go time.AfterFunc(t.renewDuration(), t.sendRenewTokenSignal)

	// renew token on signal util remote kite disconnects.
	for {
		select {
		case <-t.signalRenewToken:
			switch err := t.renewToken(); {
			case err == nil:
				go time.AfterFunc(t.renewDuration(), t.sendRenewTokenSignal)
			case err == ErrNoKitesAvailable || strings.Contains(err.Error(), "no kites found"):
				// If kite went down we're not going to renew the token,
				// as we need to dial either way.
				//
				// This case handles a situation, when kite missed
				// disconnect signal (observed to happen with XHR transport).
			default:
				t.localKite.Log.Error("token renewer: %s Cannot renew token for Kite: %s I will retry in %d seconds...",
					err, t.client.ID, retryInterval/time.Second)
				// Need to sleep here litle bit because a signal is sent
				// when an expired token is detected on incoming request.
				// This sleep prevents the signal from coming too fast.
				time.Sleep(1 * time.Second)
				go time.AfterFunc(retryInterval, t.sendRenewTokenSignal)
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

func (t *TokenRenewer) startRenewLoop() {
	// In case when t.client missed a disconnect signal (e.g. due to timeout observed
	// by the remote end), previous renewLoop will be still running.
	t.sendDisconnectSignal()

	// if we don't wait to observe previous renewLoop goroutine handle the disconnect
	// signal, we'd have a race resulting in new renewLoop goroutine handling it.
	t.renewLoopWG.Wait()

	go t.renewLoop()
}

func (t *TokenRenewer) sendRenewTokenSignal() {
	// Needs to be non-blocking because tokenRenewer may be stopped.
	select {
	case t.signalRenewToken <- struct{}{}:
	default:
	}
}

func (t *TokenRenewer) sendDisconnectSignal() {
	// Needs to be non-blocking because tokenRenewer may be stopped.
	select {
	case t.disconnect <- struct{}{}:
	default:
	}
}

// renewToken gets a new token from a kontrolClient, parses it and sets it as the token.
func (t *TokenRenewer) renewToken() error {
	renew := &protocol.Kite{
		ID: t.client.Kite.ID,
	}

	token, err := t.localKite.GetToken(renew)
	if err != nil {
		return err
	}

	if err = t.parse(token); err != nil {
		return err
	}

	t.client.authMu.Lock()
	t.client.Auth.Key = token
	t.client.authMu.Unlock()

	t.client.callOnTokenRenewHandlers(token)

	return nil
}
