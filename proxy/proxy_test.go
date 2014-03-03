package proxy

import (
	"net/url"
	"strings"
	"testing"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestProxy(t *testing.T) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	prx := New(conf.Copy(), testkeys.Public, testkeys.Private)
	prx.Kite.Config.DisableAuthentication = true // no kontrol running in test
	prx.Start()

	// Proxy kite is ready.

	kite1 := kite.New("kite1", "1.0.0")
	kite1.Config = conf.Copy()
	kite1.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})

	prxClt := kite1.NewClientString("ws://localhost:3999/kite")
	err := prxClt.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// Kite1 is connected to proxy.

	result, err := prxClt.Tell("register")
	if err != nil {
		t.Fatal(err)
	}

	proxyURL := result.MustString()

	t.Logf("Registered to proxy with URL: %s", proxyURL)

	if !strings.Contains(proxyURL, "/proxy") {
		t.Fatalf("Invalid proxy URL: %s", proxyURL)
	}

	kite2 := kite.New("kite2", "1.0.0")
	kite2.Config = conf.Copy()

	kite1remote := kite2.NewClientString(proxyURL)

	err = kite1remote.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// kite2 is connected to kite1 via proxy kite.

	result, err = kite1remote.Tell("foo")
	if err != nil {
		t.Fatal(err)
	}

	s := result.MustString()
	if s != "bar" {
		t.Fatalf("Wrong reply: %s", s)
	}
}
