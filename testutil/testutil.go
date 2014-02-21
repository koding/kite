// Package testutil provides a default Kontrol and RegServ kites for
// using in tests.
package testutil

import (
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testkeys"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
)

// WriteKiteKey writes a new kite key. (Copied and modified from regserv.go)
// If the host does not have a kite.key file kite.New() panics.
// This is a helper to put a fake key on it's location.
func WriteKiteKey() {
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
		"iss":        "testuser",                    // Issuer
		"sub":        "testuser",                    // Issued to
		"iat":        time.Now().UTC().Unix(),       // Issued At
		"aud":        hostname,                      // Hostname of registered machine
		"kontrolURL": "ws://localhost:3999/kontrol", // Kontrol URL
		"kontrolKey": testkeys.Public,               // Public key of kontrol
		"jti":        tknID.String(),                // JWT ID
	}

	key, err := token.SignedString([]byte(testkeys.Private))
	if err != nil {
		panic(err)
	}

	err = kitekey.Write(key)
	if err != nil {
		panic(err)
	}
}
