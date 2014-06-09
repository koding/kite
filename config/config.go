// Package config contains a Config struct for kites.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/koding/kite/kitekey"
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

	// Options for Server
	IP   string
	Port int

	KontrolURL  *url.URL
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
}

// New returns a new Config initialized with defaults.
func New() *Config {
	c := new(Config)
	*c = *DefaultConfig
	return c
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
		c.KontrolURL, err = url.Parse(kontrolURL)
		if err != nil {
			return err
		}
	}

	return nil
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

	// jti is used for jwt's but let's also use it for kite ID
	if id, ok := key.Claims["jti"].(string); ok {
		c.Id = id
	}

	if kontrolURL, ok := key.Claims["kontrolURL"].(string); ok {
		c.KontrolURL, err = url.Parse(kontrolURL)
		if err != nil {
			return err
		}
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
