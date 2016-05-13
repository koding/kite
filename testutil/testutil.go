// Package testutil provides a default Kontrol kites for using in tests.
package testutil

import (
	"os"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/config"
	"github.com/koding/kite/testkeys"
	"github.com/koding/logging"
	uuid "github.com/satori/go.uuid"
)

// NewKiteKey returns a new generated kite key. (Copied and modified from
// kontrol.go) If the host does not have a kite.key file kite.New() panics.
// This is a helper to put a fake key on it's location.
func NewKiteKey() *jwt.Token {
	return newKiteKey("", testkeys.Private, testkeys.Public)
}

// NewKiteKeyUsername is like NewKiteKey() but it uses the given username
// instead of using the "testuser" name
func NewKiteKeyUsername(username string) *jwt.Token {
	return newKiteKey(username, testkeys.Private, testkeys.Public)
}

func NewKiteKeyWithKeyPair(private, public string) *jwt.Token {
	return newKiteKey("", private, public)
}

func newKiteKey(username, private, public string) *jwt.Token {
	tknID := uuid.NewV4()

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	if username == "" {
		username = "testuser"
	}

	if testuser := os.Getenv("TESTKEY_USERNAME"); testuser != "" {
		username = testuser
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        "testuser",                   // Issuer
		"sub":        username,                     // Issued to
		"aud":        hostname,                     // Hostname of registered machine
		"iat":        time.Now().UTC().Unix(),      // Issued At
		"jti":        tknID.String(),               // JWT ID
		"kontrolURL": "http://localhost:4000/kite", // Kontrol URL
		"kontrolKey": public,                       // Public key of kontrol
	}

	token.Raw, err = token.SignedString([]byte(private))
	if err != nil {
		panic(err)
	}

	// verify the token
	_, err = jwt.Parse(token.Raw, func(*jwt.Token) (interface{}, error) {
		return []byte(public), nil
	})

	if err != nil {
		panic(err)
	}

	token.Valid = true
	return token

}

func NewConfig() *config.Config {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:4000/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = NewKiteKey().Raw
	return conf
}

func init() {
	// Monkey-patch default logging handler that is used by Kite.Logger
	// in order to hide logs until "-v" flag is given to "test.sh" script.
	original := logging.DefaultHandler
	logging.DefaultHandler = &quietHandler{original}
}

// quietHandler does not output any log messages until "-v" flag is given in tests.
type quietHandler struct {
	logging.Handler
}

func (h *quietHandler) Handle(rec *logging.Record) {
	if testing.Verbose() {
		h.Handler.Handle(rec)
	}
}
