package tunnelproxy

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestProxy(t *testing.T) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "ws://localhost:6666/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw
	conf.Transport = config.WebSocket // tunnel only works via WebSocket

	// start kontrol
	color.Green("Starting kontrol")
	kontrol.DefaultPort = 6666
	kon := kontrol.New(conf.Copy(), "0.1.0")

	switch os.Getenv("KONTROL_STORAGE") {
	case "etcd":
		kon.SetStorage(kontrol.NewEtcd(nil, kon.Kite.Log))
	case "postgres":
		p := kontrol.NewPostgres(nil, kon.Kite.Log)
		kon.SetStorage(p)
		kon.SetKeyPairStorage(p)
	default:
		kon.SetStorage(kontrol.NewEtcd(nil, kon.Kite.Log))
	}

	kon.AddKeyPair("", testkeys.Public, testkeys.Private)

	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	DefaultPort = 4999
	DefaultPublicHost = "localhost:4999"
	prxConf := conf.Copy()
	prxConf.DisableAuthentication = true // no kontrol running in test
	prx := New(prxConf, "0.1.0", testkeys.Public, testkeys.Private)
	prx.Start()

	log.Println("Proxy started")

	// Proxy kite is ready.
	kite1 := kite.New("kite1", "1.0.0")
	kite1.Config = conf.Copy()
	kite1.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})

	prxClt := kite1.NewClient("http://localhost:4999/kite")
	err := prxClt.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// Kite1 is connected to proxy.

	result, err := prxClt.TellWithTimeout("register", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	proxyURL := result.MustString()

	log.Printf("Registered to proxy with URL: %s", proxyURL)

	if !strings.Contains(proxyURL, "/proxy") {
		t.Fatalf("Invalid proxy URL: %s", proxyURL)
	}

	kite2 := kite.New("kite2", "1.0.0")
	kite2.Config = conf.Copy()

	kite1remote := kite2.NewClient(proxyURL)

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
}
