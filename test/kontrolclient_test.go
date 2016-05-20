package kontrolclient_test

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

var (
	conf *config.Config
	kon  *kontrol.Kontrol
)

func init() {
	conf = config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:4099/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw
	conf.ReadEnvironmentVariables()

	kontrol.DefaultPort = 4099
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
}

func TestRegisterToKontrol(t *testing.T) {
	k := setup()
	defer k.Close()

	kiteURL := &url.URL{Scheme: "http", Host: "zubuzaretta:16500", Path: "/kite"}
	go k.RegisterForever(kiteURL)

	select {
	case <-k.KontrolReadyNotify():
		kites, err := k.GetKites(&protocol.KontrolQuery{
			Username:    k.Kite().Username,
			Environment: k.Kite().Environment,
			Name:        k.Kite().Name,
		})
		if err != nil {
			t.Fatal(err)
		}

		first := kites[0]
		if first.Username != k.Kite().Username {
			t.Errorf("unexpected kite. got: %s, want: %s", first.Kite, *k.Kite())
		}
		if first.URL != "http://zubuzaretta:16500/kite" {
			t.Errorf("unexpected url: %s", first.URL)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}

func setup() *kite.Kite {
	k := kite.New("test", "1.0.0")
	k.Config = conf
	k.HandleFunc("hello", hello)
	return k
}

func hello(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
