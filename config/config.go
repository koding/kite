// Package config contains a Config struct for kites.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
)

// Options is passed to kite.New when creating new instance.
type Config struct {
	// Options for Kite
	Username              string
	Environment           string
	Region                string
	Id                    string
	KiteKey               string
	DisableAuthentication bool
	DisableConcurrency    bool
	Transport             Transport

	// Options for Server
	IP   string
	Port int

	// VerifyFunc is used to verify the public key of the signed token.
	//
	// If the pub key is not to be trusted, the function must return
	// kite.ErrKeyNotTrusted error.
	//
	// If nil, the default verify is used. By default the public key
	// is verified by calling Kontrol and the result cached for
	// VerifyTTL seconds if KontrolVerify is true. Otherwise
	// only public keys that are the same as the KontrolKey one are
	// accepted.
	VerifyFunc func(pub string) error

	// VerifyTTL is used to control time after result of a single
	// VerifyFunc's call expires.
	//
	// When <0, the result is not cached.
	//
	// When 0, the default value of 300s is used.
	VerifyTTL time.Duration

	// VerifyAudienceFunc is used to verify the audience of JWT token.
	//
	// If nil, the default audience verify function is used which
	// expects the aud to be a kite path that matches the username,
	// environment and name of the client.
	VerifyAudienceFunc func(client *protocol.Kite, aud string) error

	KontrolURL  string
	KontrolKey  string
	KontrolUser string
}

// DefaultConfig contains the default settings.
var DefaultConfig = &Config{
	Username:    "unknown",
	Environment: "unknown",
	Region:      "unknown",
	IP:          "0.0.0.0",
	Port:        0,
	Transport:   WebSocket,
}

// New returns a new Config initialized with defaults.
func New() *Config {
	c := new(Config)
	*c = *DefaultConfig
	return c
}

// NewFromKiteKey parses the given kite key file and gives a new Config value.
func NewFromKiteKey(file string) (*Config, error) {
	key, err := kitekey.ParseFile(file)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := c.ReadToken(key); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) ReadEnvironmentVariables() error {
	var err error

	if username := os.Getenv("KITE_USERNAME"); username != "" {
		c.Username = username
	}

	if environment := os.Getenv("KITE_ENVIRONMENT"); environment != "" {
		c.Environment = environment
	}

	if region := os.Getenv("KITE_REGION"); region != "" {
		c.Region = region
	}

	if ip := os.Getenv("KITE_IP"); ip != "" {
		c.IP = ip
	}

	if port := os.Getenv("KITE_PORT"); port != "" {
		c.Port, err = strconv.Atoi(port)
		if err != nil {
			return err
		}
	}

	if kontrolURL := os.Getenv("KITE_KONTROL_URL"); kontrolURL != "" {
		c.KontrolURL = kontrolURL
	}

	if transportName := os.Getenv("KITE_TRANSPORT"); transportName != "" {
		transport, ok := Transports[transportName]
		if !ok {
			return fmt.Errorf("transport '%s' doesn't exists", transportName)
		}

		c.Transport = transport
	}

	if ttl, err := time.ParseDuration(os.Getenv("KITE_VERIFY_TTL")); err == nil {
		c.VerifyTTL = ttl
	}

	return nil
}

// ReadKiteKey parsed the user's kite key and returns a new Config.
func (c *Config) ReadKiteKey() error {
	key, err := kitekey.Parse()
	if err != nil {
		return err
	}

	return c.ReadToken(key)
}

// ReadToken reads Kite Claims from JWT token and uses them to initialize Config.
func (c *Config) ReadToken(key *jwt.Token) error {
	c.KiteKey = key.Raw

	claims, ok := key.Claims.(*kitekey.KiteClaims)
	if !ok {
		return errors.New("no claims found")
	}

	c.Username = claims.Subject
	c.KontrolUser = claims.Issuer
	c.Id = claims.Id // jti is used for jwt's but let's also use it for kite ID
	c.KontrolURL = claims.KontrolURL
	c.KontrolKey = claims.KontrolKey

	return nil
}

// Copy returns a new copy of the config object.
func (c *Config) Copy() *Config {
	cloned := new(Config)
	*cloned = *c
	return cloned
}

func Get() (*Config, error) {
	c := New()
	if err := c.ReadKiteKey(); err != nil {
		return nil, err
	}
	if err := c.ReadEnvironmentVariables(); err != nil {
		return nil, err
	}
	return c, nil
}

func MustGet() *Config {
	c, err := Get()
	if err != nil {
		fmt.Printf("Cannot read kite.key: %s\n", err.Error())
		os.Exit(1)
	}
	return c
}
