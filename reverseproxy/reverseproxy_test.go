package reverseproxy

import (
	"net/url"
	"strings"
	"testing"
	"time"

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

	proxyConf := conf.Copy()
	proxyConf.DisableAuthentication = true // no kontrol running in test

	proxy := New(proxyConf)
	go proxy.Run()
	<-proxy.ReadyNotify()

	// Proxy kite is ready.
	kite1 := kite.New("kite1", "1.0.0")
	kite1.Config = conf.Copy()
	kite1.Config.DisableAuthentication = true
	kite1.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})

	kite1.Config.Port = 7777
	kite1URL := &url.URL{Scheme: "ws", Host: "localhost:7777"}
	go kite1.Run()

	<-kite1.ServerReadyNotify()

	prxClt := kite1.NewClientString("ws://localhost:3999/kite")
	err := prxClt.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// Kite1 is connected to proxy.
	result, err := prxClt.TellWithTimeout("register", 4*time.Second, kite1URL.String())
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
	result, err = kite1remote.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	s := result.MustString()
	if s != "bar" {
		t.Fatalf("Wrong reply: %s", s)
	}

	kite1.Close()
}
