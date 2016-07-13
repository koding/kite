package kontrol

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testutil"
)

var interactive = os.Getenv("TEST_INTERACTIVE") == "1"

type Config struct {
	Config       *config.Config
	Private      string
	Public       string
	RegisterFunc func(*HelloKite) error
}

func (c *Config) Register(hk *HelloKite) error {
	if c.RegisterFunc != nil {
		return c.RegisterFunc(hk)
	}

	if err := hk.Kite.RegisterForever(hk.URL); err != nil {
		hk.Kite.Close()
		return err
	}

	if _, err := hk.WaitRegister(15 * time.Second); err != nil {
		hk.Kite.Close()
		return err
	}

	return nil
}

func startKontrol(pem, pub string, port int) (*Kontrol, *Config) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = fmt.Sprintf("http://localhost:%d/kite", port)
	conf.KontrolKey = pub
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewToken("testuser", pem, pub).Raw
	conf.ReadEnvironmentVariables()

	DefaultPort = port
	kon := New(conf.Copy(), "1.0.0")
	// kon.Kite.SetLogLevel(kite.DEBUG)

	switch os.Getenv("KONTROL_STORAGE") {
	case "etcd":
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	case "postgres":
		p := NewPostgres(nil, kon.Kite.Log)
		kon.SetStorage(p)
		kon.SetKeyPairStorage(p)
	default:
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	}

	kon.AddKeyPair("", pub, pem)

	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	return kon, &Config{
		Config:  conf,
		Private: pem,
		Public:  pub,
	}
}

func klose(clients []*kite.Client) {
	for _, c := range clients {
		c.Close()
	}
}

type HelloKite struct {
	Kite  *kite.Kite
	URL   *url.URL
	Token bool

	clients map[string]*kite.Client
	regs    chan *protocol.RegisterResult
	toks    chan struct{}
	renew   chan struct{}
}

func NewHelloKite(name string, conf *Config) (*HelloKite, error) {
	k := kite.New(name, "1.0.0")
	k.Config = conf.Config.Copy()
	k.Config.Port = 0
	k.Config.KiteKey = testutil.NewToken(name, conf.Private, conf.Public).Raw
	// k.SetLogLevel(kite.DEBUG)

	k.HandleFunc("hello", func(r *kite.Request) (interface{}, error) {
		return fmt.Sprintf("%s says hello", name), nil
	})

	go k.Run()
	<-k.ServerReadyNotify()

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", k.Port()),
		Path:   "/kite",
	}

	hk := &HelloKite{
		Kite:    k,
		URL:     u,
		clients: make(map[string]*kite.Client),
		regs:    make(chan *protocol.RegisterResult, 16),
		toks:    make(chan struct{}, 16),
		renew:   make(chan struct{}, 16),
	}

	hk.Kite.OnRegister(hk.onRegister)

	if err := conf.Register(hk); err != nil {
		return nil, err
	}

	return hk, nil
}

func (hk *HelloKite) onRegister(reg *protocol.RegisterResult) {
	hk.regs <- reg
}

func (hk *HelloKite) WaitRegister(timeout time.Duration) (*protocol.RegisterResult, error) {
	select {
	case reg := <-hk.regs:
		return reg, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("waiting for register timed out after %s", timeout)
	}
}

func (hk *HelloKite) onTokenExpire() {
	hk.toks <- struct{}{}
}

func (hk *HelloKite) onTokenRenew(string) {
	hk.renew <- struct{}{}
}

func (hk *HelloKite) WaitTokenExpired(timeout time.Duration) error {
	if !hk.Token {
		return errors.New("kite is not authenticated with token")
	}

	select {
	case <-hk.toks:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("waiting for token to expire timed out after %s", timeout)
	}
}

func (hk *HelloKite) WaitTokenRenew(timeout time.Duration) error {
	if !hk.Token {
		return errors.New("kite is not authenticated with token")
	}

	select {
	case <-hk.renew:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("waiting for token to renew timed out after %s", timeout)
	}
}

func (hk *HelloKite) Hello(remote *HelloKite) (string, error) {
	return hk.hello(remote, false)
}

func (hk *HelloKite) hello(remote *HelloKite, force bool) (string, error) {
	const timeout = 10 * time.Second

	c, ok := hk.clients[remote.Kite.Id]
	if !ok || force {
		if hk.Token {
			query := &protocol.KontrolQuery{
				ID: remote.Kite.Id,
			}

			kites, err := hk.Kite.GetKites(query)
			if err != nil {
				return "", fmt.Errorf("%s: %+v", err, query)
			}

			if len(kites) != 1 {
				return "", fmt.Errorf("%s: expected to get 1 kite, got %d", remote.Kite.Id, len(kites))
			}

			c = kites[0]
			c.OnTokenExpire(hk.onTokenExpire)
			c.OnTokenRenew(hk.onTokenRenew)
		} else {
			c = hk.Kite.NewClient(remote.URL.String())
			c.Auth = &kite.Auth{
				Type: "kiteKey",
				Key:  hk.Kite.KiteKey(),
			}
		}

		if err := c.DialTimeout(timeout); err != nil {
			return "", err
		}

		hk.clients[remote.Kite.Id] = c
	}

	res, err := c.TellWithTimeout("hello", timeout)
	if err != nil {
		// TODO(rjeczalik): remove "timeout" - see comment in (*Client).sendHub method
		if e, ok := err.(*kite.Error); ok && (e.Type == "disconnect" || e.Type == "timeout") && !force {
			return hk.hello(remote, true)
		}

		return "", err
	}

	s, err := res.String()
	if err != nil {
		return "", err
	}

	return s, nil
}

func (hk *HelloKite) Close() error {
	for _, c := range hk.clients {
		c.Close()
	}

	hk.Kite.Close()

	return nil
}

type HelloKites map[*HelloKite]*HelloKite

func Call(kitePairs HelloKites) error {
	merr := &multiError{}

	for local, remote := range kitePairs {
		call := fmt.Sprintf("%s -> %s", local.Kite.Kite().Name, remote.Kite.Kite().Name)

		got, err := local.Hello(remote)
		if err != nil {
			err = fmt.Errorf("%s: error calling: %s", call, err)
			merr.err = append(merr.err, err)
			continue
		}

		if want := fmt.Sprintf("%s says hello", remote.Kite.Kite().Name); got != want {
			err = fmt.Errorf("%s: invalid response: got %q, want %q", call, got, want)
			merr.err = append(merr.err, err)
			continue
		}
	}

	if len(merr.err) != 0 {
		return merr
	}

	return nil
}

func WaitTillConnected(conf *Config, timeout time.Duration, hks ...*HelloKite) error {
	k := kite.New("WaitTillConnected", "1.0.0")
	k.Config = conf.Config.Copy()

	t := time.After(timeout)

	for {
		select {
		case <-t:
			return fmt.Errorf("timed out waiting for %v after %s", hks, timeout)
		default:
			kites, err := k.GetKites(&protocol.KontrolQuery{
				Version: "1.0.0",
			})
			if err != nil && err != kite.ErrNoKitesAvailable {
				return err
			}

			notReady := make(map[string]struct{}, len(hks))
			for _, hk := range hks {
				notReady[hk.Kite.Kite().ID] = struct{}{}
			}

			for _, kite := range kites {
				delete(notReady, kite.Kite.ID)
			}

			if len(notReady) == 0 {
				return nil
			}
		}
	}
}

func pause(args ...interface{}) {
	if testing.Verbose() && interactive {
		fmt.Println("[PAUSE]", fmt.Sprint(args...), fmt.Sprintf(`("kill -1 %d" to continue)`, os.Getpid()))
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGHUP)
		<-ch
		signal.Stop(ch)
	}
}
