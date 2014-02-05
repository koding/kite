package regserv

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
	"kite"
	"time"
)

// RegServ is a registration kite. Users can register their machines by
// running "kite register" command.
type RegServ struct {
	*kite.Kite
	backend Backend
}

func New(backend Backend) *RegServ {
	regserv := &RegServ{
		backend: backend,
	}
	opts := &kite.Options{
		Kitename:              "regserv",
		Version:               "0.0.1",
		Port:                  "8080",
		Path:                  "/regserv",
		Region:                "localhost",
		Environment:           "development",
		DisableAuthentication: true,
	}
	regserv.Kite = kite.New(opts)
	regserv.HandleFunc("register", regserv.handleRegister)
	return regserv
}

type Backend interface {
	Issuer() string
	KontrolURL() string
	PublicKey() string
	PrivateKey() string

	// Authenticate the user and return username.
	Authenticate(r *kite.Request) (string, error)
}

func (s *RegServ) handleRegister(r *kite.Request) (interface{}, error) {
	var args struct {
		Hostname string
	}
	r.Args.One().MustUnmarshal(&args)

	tknID, err := uuid.NewV4()
	if err != nil {
		return nil, errors.New("cannot generate a token")
	}

	username, err := s.backend.Authenticate(r)
	if err != nil {
		return nil, errors.New("cannot authenticate user")
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))
	token.Claims["iss"] = s.backend.Issuer()            // Issuer
	token.Claims["sub"] = username                      // Subject
	token.Claims["iat"] = time.Now().UTC().Unix()       // Issued At
	token.Claims["hostname"] = args.Hostname            // Hostname of registered machine
	token.Claims["kontrolURL"] = s.backend.KontrolURL() // Kontrol URL
	token.Claims["jti"] = tknID.String()                // JWT ID

	signed, err := token.SignedString([]byte(s.backend.PrivateKey()))
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"kite.key":    signed,
		"kontrol.key": s.backend.PublicKey(),
	}, nil
}
