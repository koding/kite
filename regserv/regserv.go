// Package regserv implements a registration server kite. Users can register
// to a kite infrastructure by running "kite register" command.
package regserv

import (
	"errors"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/registration"
	"github.com/nu7hatch/gouuid"
)

const (
	RegservVersion = "0.0.2"
)

// RegServ is a registration kite. Users can register their machines by
// running "kite register" command.
type RegServ struct {
	Kite         *kite.Kite
	Authenticate func(r *kite.Request) error
	publicKey    string
	privateKey   string
}

func New(conf *config.Config, version, pubKey, privKey string) *RegServ {
	k := kite.New("regserv", version)
	k.Config = conf
	r := &RegServ{
		Kite:       k,
		publicKey:  pubKey,
		privateKey: privKey,
	}
	k.HandleFunc("register", r.handleRegister)
	return r
}

func (s *RegServ) Run() {
	reg := registration.New(s.Kite)

	s.Kite.Start()
	go reg.RegisterToProxyAndKontrol()

	<-s.Kite.CloseNotify()
}

// RegisterSelf registers this host and writes a key to ~/.kite/kite.key
func (s *RegServ) RegisterSelf() error {
	key, err := s.register(s.Kite.Config.Username)
	if err != nil {
		return err
	}
	return kitekey.Write(key)
}

func (s *RegServ) handleRegister(r *kite.Request) (interface{}, error) {
	if s.Authenticate != nil {
		if err := s.Authenticate(r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	return s.register(r.Client.Kite.Username)
}

func (s *RegServ) register(username string) (kiteKey string, err error) {
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("cannot generate a token")
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        s.Kite.Kite().Username,            // Issuer
		"sub":        username,                          // Subject
		"iat":        time.Now().UTC().Unix(),           // Issued At
		"jti":        tknID.String(),                    // JWT ID
		"kontrolURL": s.Kite.Config.KontrolURL.String(), // Kontrol URL
		"kontrolKey": strings.TrimSpace(s.publicKey),    // Public key of kontrol
	}

	s.Kite.Log.Info("Registered user: %s", username)

	return token.SignedString([]byte(s.privateKey))
}
