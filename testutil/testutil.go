// Package testutil provides a default Kontrol and RegServ kites for
// using in tests.
package testutil

import (
	"net/url"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/config"
	"github.com/koding/kite/testkeys"
	"github.com/nu7hatch/gouuid"
)

// NewKiteKey returns a new generated kite key. (Copied and modified from regserv.go)
// If the host does not have a kite.key file kite.New() panics.
// This is a helper to put a fake key on it's location.
func NewKiteKey() *jwt.Token {
	tknID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        "testuser",              // Issuer
		"sub":        "testuser",              // Issued to
		"aud":        hostname,                // Hostname of registered machine
		"iat":        time.Now().UTC().Unix(), // Issued At
		"jti":        tknID.String(),          // JWT ID
		"kontrolURL": "ws://localhost:4000",   // Kontrol URL
		"kontrolKey": testkeys.Public,         // Public key of kontrol
	}

	token.Raw, err = token.SignedString([]byte(testkeys.Private))
	if err != nil {
		panic(err)
	}

	token.Valid = true
	return token
}

func NewConfig() *config.Config {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = NewKiteKey().Raw
	return conf
}
