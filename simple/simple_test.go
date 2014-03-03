package simple

import (
	"io/ioutil"
	"net/url"
	"os"
	"testing"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestSimple(t *testing.T) {
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
	kon.ClearKites()

	prx := proxy.New(conf.Copy(), testkeys.Public, testkeys.Private)
	prx.Start()

	s := New("hello", "1.0.0")
	s.HandleFunc("hello", hello)
	s.Start()

	<-s.Registration.ReadyNotify()
}

func hello(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
