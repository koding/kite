// Package testutil provides a default Kontrol kites for using in tests.
package testutil

import (
	"os"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testkeys"
	"github.com/koding/logging"
	uuid "github.com/satori/go.uuid"
)

// NewKiteKey returns a new generated kite key. (Copied and modified from
// kontrol.go) If the host does not have a kite.key file kite.New() panics.
// This is a helper to put a fake key on it's location.
func NewKiteKey() *jwt.Token {
	return NewToken("", testkeys.Private, testkeys.Public)
}

// NewKiteKeyUsername is like NewKiteKey() but it uses the given username
// instead of using the "testuser" name
func NewKiteKeyUsername(username string) *jwt.Token {
	return NewToken(username, testkeys.Private, testkeys.Public)
}

func NewKiteKeyWithKeyPair(private, public string) *jwt.Token {
	return NewToken("", private, public)
}

// NewToken creates new JWT token for the gien username. It embedds the given
// public key as kontrolKey and signs the token with the private one.
func NewToken(username, private, public string) *jwt.Token {
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

	claims := &kitekey.KiteClaims{
		StandardClaims: jwt.StandardClaims{
			Issuer:   "testuser",
			Subject:  username,
			Audience: hostname,
			IssuedAt: time.Now().UTC().Unix(),
			Id:       tknID.String(),
		},
		KontrolKey: public,
		KontrolURL: "http://localhost:4000/kite",
	}

	token := jwt.NewWithClaims(jwt.GetSigningMethod("RS256"), claims)

	rsaPrivate, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(private))
	if err != nil {
		panic(err)
	}

	token.Raw, err = token.SignedString(rsaPrivate)
	if err != nil {
		panic(err)
	}

	// verify the token
	_, err = jwt.ParseWithClaims(token.Raw, claims, func(*jwt.Token) (interface{}, error) {
		return jwt.ParseRSAPublicKeyFromPEM([]byte(public))
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
	original.SetLevel(logging.DEBUG)
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
