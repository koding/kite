package regserv

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"github.com/nu7hatch/gouuid"
	"kite"
	"kite/kitekey"
	"os"
	"time"
)

const version = "0.0.1"

// RegServ is a registration kite. Users can register their machines by
// running "kite register" command.
type RegServ struct {
	Environment string
	Region      string
	PublicIP    string
	Port        string
	backend     Backend
	kite        *kite.Kite
}

func New(backend Backend) *RegServ {
	return &RegServ{
		Environment: "production",
		Region:      "localhost",
		backend:     backend,
	}
}

func (s *RegServ) Run() {
	if s.Environment == "" || s.Region == "" || s.PublicIP == "" || s.Port == "" {
		panic("RegServ is not initialized properly")
	}

	_, err := kitekey.Parse()
	if err != nil {
		s.registerSelf() // Need to do this before creating new kite
	}

	// Create a kite and run it.
	opts := &kite.Options{
		Kitename:    "regserv",
		Version:     version,
		Environment: s.Environment,
		Region:      s.Region,
		PublicIP:    s.PublicIP,
		Port:        s.Port,
		Path:        "/regserv",
		DisableAuthentication: true,
	}
	s.kite = kite.New(opts)
	s.kite.HandleFunc("register", s.handleRegister)
	s.kite.Run()
}

// registerSelf registers this host and writes a key to ~/.kite/kite.key
func (s *RegServ) registerSelf() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	key, err := s.register(s.backend.Issuer(), hostname)
	if err != nil {
		return err
	}
	return kitekey.Write(key)
}

// Backend is the interface that is passed to New() function.
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

	username, err := s.backend.Authenticate(r)
	if err != nil {
		return nil, errors.New("cannot authenticate user")
	}

	return s.register(username, args.Hostname)
}

func (s *RegServ) register(username, hostname string) (kiteKey string, err error) {
	tknID, err := uuid.NewV4()
	if err != nil {
		return "", errors.New("cannot generate a token")
	}

	token := jwt.New(jwt.GetSigningMethod("RS256"))

	token.Claims = map[string]interface{}{
		"iss":        s.backend.Issuer(),      // Issuer
		"sub":        username,                // Subject
		"iat":        time.Now().UTC().Unix(), // Issued At
		"hostname":   hostname,                // Hostname of registered machine
		"kontrolURL": s.backend.KontrolURL(),  // Kontrol URL
		"kontrolKey": s.backend.PublicKey(),   // Public key of kontrol
		"jti":        tknID.String(),          // JWT ID
	}

	return token.SignedString([]byte(s.backend.PrivateKey()))
}
