package regserv

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testkeys"
	"testing"
)

type testBackend struct{}

func (b testBackend) Username() string                             { return "testuser" }
func (b testBackend) KontrolURL() string                           { return "ws://localhost:3999/kontrol" }
func (b testBackend) PublicKey() string                            { return testkeys.Public }
func (b testBackend) PrivateKey() string                           { return testkeys.Private }
func (b testBackend) Authenticate(r *kite.Request) (string, error) { return "testuser", nil }

func TestRegister(t *testing.T) {
	regserv := New(testBackend{})
	regserv.Environment = "testing"
	regserv.Region = "localhost"
	regserv.PublicIP = "127.0.0.1"
	regserv.Port = "8079"

	key, err := regserv.register("foo", "bar")
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	token, err := jwt.Parse(key, kitekey.GetKontrolKey)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	if username := token.Claims["sub"].(string); username != "foo" {
		t.Errorf("invalid username: %s", username)
		return
	}

	if hostname := token.Claims["aud"].(string); hostname != "bar" {
		t.Errorf("invalid hostname: %s", hostname)
		return
	}
}
