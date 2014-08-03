package pool

import (
	"net/url"
	"strconv"
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

func TestPool(t *testing.T) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:5555/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	kontrol.DefaultPort = 5555
	kon := kontrol.New(conf.Copy(), "0.1.0", testkeys.Public, testkeys.Private)
	go kon.Run()
	<-kon.Kite.ServerReadyNotify()
	// defer kon.Close()

	// prx := tunnelproxy.New(conf.Copy(), "0.1.0", testkeys.Public, testkeys.Private)
	// prx.Start()
	// defer prx.Close()

	foo := kite.New("foo", "1.0.0")
	foo.Config = conf.Copy()

	query := protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        "bar",
	}

	p := New(foo, query)
	p.Start()
	// defer p.Close()

	for i := 0; i < 2; i++ {
		bar := kite.New("bar", "1.0.0")
		bar.Config = conf.Copy()
		bar.Config.Port = 6760 + i
		go bar.Run()
		<-bar.ServerReadyNotify()

		go bar.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(bar.Config.Port+i), Path: "/kite"})
		defer bar.Close()
		<-bar.KontrolReadyNotify()
	}

	// We must wait for a until the pool receives events from kontrol.
	time.Sleep(2 * time.Second)

	if p.Len() != 2 {
		t.Fatalf("expected 2 kited, found: %d", p.Len())
	}
}
