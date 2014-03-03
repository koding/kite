package pool

import (
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/simple"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestPool(t *testing.T) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	kon := kontrol.New(conf.Copy(), testkeys.Public, testkeys.Private)
	kon.DataDir, _ = ioutil.TempDir("", "")
	defer os.RemoveAll(kon.DataDir)
	kon.Start()
	// defer kon.Close()

	prx := proxy.New(conf.Copy(), testkeys.Public, testkeys.Private)
	prx.Start()
	// defer prx.Close()

	foo := kite.New("foo", "1.0.0")
	foo.Config = conf.Copy()
	konClient := kontrolclient.New(foo)
	connected, err := konClient.DialForever()
	if err != nil {
		t.Fatal(err)
	}
	<-connected

	query := protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        "bar",
	}

	p := New(konClient, query)
	p.Start()
	// defer p.Close()

	for i := 0; i < 2; i++ {
		bar := simple.New("bar", "1.0.0")
		bar.Config = conf.Copy()
		bar.Start()
		defer bar.Close()
		<-bar.Registration.ReadyNotify()
	}

	// We must wait for a until the pool receives events from kontrol.
	time.Sleep(time.Second)

	if len(p.Kites) != 2 {
		t.Fatalf("expected 2 kited, found: %d", len(p.Kites))
	}
}
