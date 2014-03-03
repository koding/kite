package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/koding/kite/kitekey"
)

// Options is passed to kite.New when creating new instance.
type Config struct {
	// Options for Kite
	Username              string
	Environment           string
	Region                string
	KiteKey               string
	DisableAuthentication bool

	// Options for Server
	IP                 string
	Port               int
	DisableConcurrency bool

	KontrolURL  *url.URL
	KontrolKey  string
	KontrolUser string
}

var defaultConfig = &Config{
	Username:    "unknown",
	Environment: "unknown",
	Region:      "unknown",
	IP:          "0.0.0.0",
	Port:        0,
}

// New returns a new Config initialized with defaults.
func New() *Config {
	c := new(Config)
	*c = *defaultConfig
	return c
}

func (c *Config) ReadEnvironmentVariables() {
	if environment := os.Getenv("KITE_ENVIRONMENT"); environment != "" {
		c.Environment = environment
	}

	if region := os.Getenv("KITE_REGION"); region != "" {
		c.Region = region
	}
}

// ReadKiteKey parsed the user's kite key and returns a new Config.
func (c *Config) ReadKiteKey() error {
	key, err := kitekey.Parse()
	if err != nil {
		return err
	}

	c.KiteKey = key.Raw

	if username, ok := key.Claims["sub"].(string); ok {
		c.Username = username
	}

	if kontrolUser, ok := key.Claims["iss"].(string); ok {
		c.KontrolUser = kontrolUser
	}

	kontrolURL := os.Getenv("KITE_KONTROL_URL")
	if kontrolURL == "" {
		var ok bool
		if kontrolURL, ok = key.Claims["kontrolURL"].(string); !ok {
			return errors.New("kontrolURL not found in kite.key")
		}
	}

	c.KontrolURL, err = url.Parse(kontrolURL)
	if err != nil {
		return err
	}

	if kontrolKey, ok := key.Claims["kontrolKey"].(string); ok {
		c.KontrolKey = kontrolKey
	}

	return nil
}

// Copy returns a new copy of the config object.
func (c *Config) Copy() *Config {
	cloned := new(Config)
	*cloned = *c
	if c.KontrolURL != nil {
		*cloned.KontrolURL = *c.KontrolURL
	}
	return cloned
}

func Get() (*Config, error) {
	c := New()
	err := c.ReadKiteKey()
	if err != nil {
		return nil, err
	}
	c.ReadEnvironmentVariables()
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
