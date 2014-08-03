package kontrolclient_test

import (
	"net/url"
	// "os"
	// "strings"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
	// "github.com/koding/kite/tunnelproxy"
)

var (
	conf *config.Config
	kon  *kontrol.Kontrol
	// prx  *tunnelproxy.Proxy
)

func init() {
	conf = config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:4099/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	kontrol.DefaultPort = 4099
	kon := kontrol.New(conf.Copy(), "0.1.0", testkeys.Public, testkeys.Private)
	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	// prx := tunnelproxy.New(conf.Copy(), "0.1.0", testkeys.Public, testkeys.Private)
	// prx.Kite.Config.DisableAuthentication = true
	// prx.Start()
}

func TestRegisterToKontrol(t *testing.T) {
	k := setup()
	defer k.Close()

	kiteURL := &url.URL{Scheme: "http", Host: "zubuzaretta:16500", Path: "/kite"}
	go k.RegisterForever(kiteURL)

	select {
	case <-k.KontrolReadyNotify():
		kites, err := k.GetKites(protocol.KontrolQuery{
			Username:    k.Kite().Username,
			Environment: k.Kite().Environment,
			Name:        k.Kite().Name,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(kites) != 1 {
			t.Fatalf("unexpected result: %+v", kites)
		}

		first := kites[0]
		if first.Kite != *k.Kite() {
			t.Errorf("unexpected kite key: %s", first.Kite)
		}
		if first.URL != "http://zubuzaretta:16500/kite" {
			t.Errorf("unexpected url: %s", first.URL)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// func TestRegisterToTunnel(t *testing.T) {
// 	k := setup()
// 	defer k.Close()

// 	go k.RegisterToTunnel()

// 	select {
// 	case <-k.KontrolReadyNotify():
// 	case <-time.After(10 * time.Second):
// 		t.Fatal("timeout")
// 	}
// }

// func TestRegisterToTunnelAndKontrol(t *testing.T) {
// 	k := setup()
// 	defer k.Close()

// 	go k.RegisterToTunnel()

// 	select {
// 	case <-k.KontrolReadyNotify():
// 		kites, err := k.GetKites(protocol.KontrolQuery{
// 			Username:    k.Kite().Username,
// 			Environment: k.Kite().Environment,
// 			Name:        k.Kite().Name,
// 		})
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		if len(kites) != 1 {
// 			t.Fatalf("unexpected result: %+v", kites)
// 		}
// 		first := kites[0]
// 		if first.Kite != *k.Kite() {
// 			t.Errorf("unexpected kite key: %s", first.Kite)
// 		}
// 		if !strings.Contains(first.WSConfig.Location.String(), "/proxy") {
// 			t.Errorf("unexpected url: %s", first.WSConfig.Location.String())
// 		}
// 	case <-time.After(2 * time.Second):
// 		t.Fatal("timeout")
// 	}
// }

func setup() *kite.Kite {
	k := kite.New("test", "1.0.0")
	k.Config = conf
	k.HandleFunc("hello", hello)
	return k
}

func hello(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
