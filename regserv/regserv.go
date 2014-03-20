// Package regserv implements a registration server kite. Users can register
// to a kite infrastructure by running "kite register" command.
package regserv

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/registration"
	"github.com/koding/kite/server"
	"github.com/nu7hatch/gouuid"
)

const Version = "0.0.2"

// RegServ is a registration kite. Users can register their machines by
// running "kite register" command.
type RegServ struct {
	Server       *server.Server
	Authenticate func(r *kite.Request) error
	publicKey    string
	privateKey   string
}

func New(conf *config.Config, pubKey, privKey string) *RegServ {
	k := kite.New("regserv", Version)
	k.Config = conf
	r := &RegServ{
		Server:     server.New(k),
		publicKey:  pubKey,
		privateKey: privKey,
	}
	k.HandleFunc("register", r.handleRegister)
	return r
}

func (s *RegServ) Run() {
	kon := kontrolclient.New(s.Server.Kite)
	reg := registration.New(kon)

	connected, err := kon.DialForever()
	if err != nil {
		s.Server.Log.Fatal(err.Error())
	}
	s.Server.Start()
	go func() {
		<-connected
		reg.RegisterToProxyAndKontrol()
	}()

	<-s.Server.CloseNotify()
}

// RegisterSelf registers this host and writes a key to ~/.kite/kite.key
func (s *RegServ) RegisterSelf() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	key, err := s.register(s.Server.Config.Username, hostname)
	if err != nil {
		return err
	}
	return kitekey.Write(key)
}

func (s *RegServ) handleRegister(r *kite.Request) (interface{}, error) {
	var args struct {
		Hostname string
	}
	r.Args.One().MustUnmarshal(&args)

	if s.Authenticate != nil {
		if err := s.Authenticate(r); err != nil {
			return nil, errors.New("cannot authenticate user")
		}
	}

	return s.register(r.Client.Kite.Username, args.Hostname)
}

func (s *RegServ) register(username, hostname string) (kiteKey string, err error) {
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("cannot generate a token")
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        s.Server.Kite.Kite().Username,       // Issuer
		"sub":        username,                            // Subject
		"aud":        hostname,                            // Hostname of registered machine
		"iat":        time.Now().UTC().Unix(),             // Issued At
		"jti":        tknID.String(),                      // JWT ID
		"kontrolURL": s.Server.Config.KontrolURL.String(), // Kontrol URL
		"kontrolKey": strings.TrimSpace(s.publicKey),      // Public key of kontrol
	}

	s.Server.Log.Info("Registered user: %s", username)

	return token.SignedString([]byte(s.privateKey))
}
