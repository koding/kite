package regserv

import (
	"testing"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestRegister(t *testing.T) {
	regserv := New(testutil.NewConfig(), testkeys.Public, testkeys.Private)

	key, err := regserv.register("foo")
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
}
