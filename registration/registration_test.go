package registration

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

var kon *kontrol.Kontrol

func init() {
	kon := kontrol.New(testkeys.Public, testkeys.Private)
	kon.DataDir = os.TempDir()
	kon.Start()
}

func TestRegisterToKontrol(t *testing.T) {
	c := config.New()
	c.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	c.KontrolKey = testkeys.Public
	c.KontrolUser = "testuser"
	c.KiteKey = testutil.NewKiteKey().Raw

	k := kite.New("test", "1.0.0")
	k.Config = c
	k.HandleFunc("hello", hello)

	konclient := kontrolclient.New(k)
	err := konclient.Dial()
	if err != nil {
		t.Fatal(err)
	}

	reg := New(konclient)
	kiteURL := &url.URL{Scheme: "ws", Host: "zubuzaretta:16500"}

	select {
	case registeredURL := <-reg.RegisterToKontrol(kiteURL):
		if *registeredURL != *kiteURL {
			t.Fatal("invalid url")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestRegisterToProxy(t *testing.T)           {}
func TestRegisterToProxyAndKontrol(t *testing.T) {}

func hello(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
