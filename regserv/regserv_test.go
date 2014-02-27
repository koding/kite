package regserv

import (
	"testing"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testkeys"
)

func TestRegister(t *testing.T) {
	conf := config.New()
	regserv := New(conf, testkeys.Public, testkeys.Private)

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
