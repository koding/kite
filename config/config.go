// Package config contains a Config struct for kites.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"time"

	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

// the implementation of New() doesn't have any error to be returned yet it
// returns, so it's totally safe to neglect the error
var CookieJar, _ = cookiejar.New(nil)

// Options is passed to kite.New when creating new instance.
type Config struct {
	// Options for Kite
	Username              string    // Username to set when registering to Kontrol.
	Environment           string    // Kite environment to set when registering to Kontrol.
	Region                string    // Kite region to set when registering to Kontrol.
	Id                    string    // Kite ID to use when registering to Kontrol.
	KiteKey               string    // The kite.key value to use for "kiteKey" authentication.
	DisableAuthentication bool      // Do not require authentication for requests.
	DisableConcurrency    bool      // Do not process messages concurrently.
	Transport             Transport // SockJS transport to use.

	IP   string // IP of the kite server.
	Port int    // Port number of the kite server.

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

	// SockJS server / client connection configuration details.

	// XHR is a HTTP client used for polling on responses for a XHR transport.
	//
	// Required.
	XHR *http.Client

	// Timeout specified max time waiting for the following operations to complete:
	//
	//   - polling on an XHR connection
	//   - default timeout for certain kite requests (Kontrol API)
	//   - HTTP heartbeats and register method
	//
	// NOTE: Ensure the Timeout is higher than SockJS.HeartbeatDelay, otherwise
	// XHR connections may get randomly closed.
	//
	// TODO(rjeczalik): Make kite heartbeats configurable as well.
	Timeout time.Duration

	// Client is a HTTP client used for issuing HTTP register request and
	// HTTP heartbeats.
	Client *http.Client

	// Websocket is used for creating a client for a websocket transport.
	//
	// If custom one is used, ensure any complemenrary field is also
	// set in sockjs.WebSocketUpgrader value (for server connections).
	//
	// Required.
	Websocket *websocket.Dialer

	// SockJS are used to configure SockJS handler.
	//
	// Required.
	SockJS *sockjs.Options

	// Serve is serving HTTP requests using handler on requests
	// comming from the given listener.
	//
	// If Serve is nil, http.Serve is used by default.
	Serve func(net.Listener, http.Handler) error

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
	Transport:   Auto,
	Timeout:     15 * time.Second,
	XHR: &http.Client{
		Jar: CookieJar,
	},
	Client: &http.Client{
		Timeout: 15 * time.Second,
		Jar:     CookieJar,
	},
	Websocket: &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		Jar:              CookieJar,
	},
	SockJS: &sockjs.Options{
		Websocket:       sockjs.DefaultOptions.Websocket,
		JSessionID:      sockjs.DefaultOptions.JSessionID,
		SockJSURL:       sockjs.DefaultOptions.SockJSURL,
		HeartbeatDelay:  10 * time.Second, // better fit for AWS ELB; empirically picked
		DisconnectDelay: 10 * time.Second, // >= Timeout
		ResponseLimit:   sockjs.DefaultOptions.ResponseLimit,
	},
}

// New returns a new Config initialized with defaults.
func New() *Config {
	return DefaultConfig.Copy()
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

	if timeout, err := time.ParseDuration(os.Getenv("KITE_TIMEOUT")); err == nil {
		c.Timeout = timeout
		c.Client.Timeout = timeout
	}

	if timeout, err := time.ParseDuration(os.Getenv("KITE_HANDSHAKE_TIMEOUT")); err == nil {
		c.Websocket.HandshakeTimeout = timeout
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
	copy := *c

	if c.XHR != nil {
		xhr := *copy.XHR
		copy.XHR = &xhr
	}

	if c.Client != nil {
		client := *copy.Client
		copy.Client = &client
	}

	if c.Websocket != nil {
		ws := *copy.Websocket
		copy.Websocket = &ws
	}

	return &copy
}
