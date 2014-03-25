package registration_test

import (
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/registration"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

var (
	conf *config.Config
	kon  *kontrol.Kontrol
	prx  *proxy.Proxy
)

func init() {
	conf = config.New()
	conf.Username = "testuser"
	conf.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	kon := kontrol.New(conf.Copy(), testkeys.Public, testkeys.Private)
	kon.DataDir, _ = ioutil.TempDir("", "")
	defer os.RemoveAll(kon.DataDir)
	kon.Start()

	prx := proxy.New(conf.Copy(), testkeys.Public, testkeys.Private)
	prx.Kite.Config.DisableAuthentication = true
	prx.Start()
}

func TestRegisterToKontrol(t *testing.T) {
	k, clt, reg := setup()
	defer clt.Close()

	kiteURL := &url.URL{Scheme: "ws", Host: "zubuzaretta:16500"}

	go reg.RegisterToKontrol(kiteURL)

	select {
	case <-reg.ReadyNotify():
		kites, err := clt.GetKites(protocol.KontrolQuery{
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
		if first.URL.String() != "ws://zubuzaretta:16500" {
			t.Errorf("unexpected url: %s", first.URL.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRegisterToProxy(t *testing.T) {
	_, clt, reg := setup()
	defer clt.Close()

	go reg.RegisterToProxy()

	select {
	case <-reg.ReadyNotify():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRegisterToProxyAndKontrol(t *testing.T) {
	k, clt, reg := setup()
	defer clt.Close()

	go reg.RegisterToProxyAndKontrol()

	select {
	case <-reg.ReadyNotify():
		kites, err := clt.GetKites(protocol.KontrolQuery{
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
		if !strings.Contains(first.URL.String(), "/proxy") {
			t.Errorf("unexpected url: %s", first.URL.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func setup() (*kite.Kite, *kontrolclient.KontrolClient, *registration.Registration) {

	k := kite.New("test", "1.0.0")
	k.Config = conf
	k.HandleFunc("hello", hello)

	konclient := kontrolclient.New(k)
	err := konclient.Dial()
	if err != nil {
		panic(err)
	}

	return k, konclient, registration.New(konclient)
}

func hello(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
