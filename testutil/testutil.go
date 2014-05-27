// Package testutil provides a default Kontrol kites for using in tests.
package testutil

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/config"
	"github.com/koding/kite/testkeys"
	"github.com/koding/logging"
	"github.com/nu7hatch/gouuid"
)

// NewKiteKey returns a new generated kite key. (Copied and modified from
// kontrol.go) If the host does not have a kite.key file kite.New() panics.
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

	username := "testuser"
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
		"kontrolKey": testkeys.Public,              // Public key of kontrol
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
	conf.KontrolURL = &url.URL{Scheme: "http", Host: "localhost:4000", Path: "/kite"}
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
