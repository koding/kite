package reverseproxy

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestProxy(t *testing.T) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = &url.URL{Scheme: "http", Host: "localhost:4000", Path: "/kite"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	// start kontrol
	color.Green("Starting kontrol")
	kon := kontrol.New(conf.Copy(), "0.1.0", testkeys.Public, testkeys.Private)
	kon.DataDir, _ = ioutil.TempDir("", "")
	defer os.RemoveAll(kon.DataDir)
	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	// start proxy
	color.Green("Starting Proxy and registering to Kontrol")
	proxyConf := conf.Copy()
	proxyConf.Port = 3999
	proxy := New(proxyConf)
	proxy.PublicHost = "localhost"
	proxy.Scheme = "http"
	go proxy.Run()
	<-proxy.ReadyNotify()

	proxyRegisterURL := &url.URL{
		Scheme: proxy.Scheme,
		Host:   proxy.PublicHost + ":" + strconv.Itoa(proxy.Port),
		Path:   "/kite",
	}

	fmt.Printf("proxyRegisterURL %+v\n", proxyRegisterURL)

	_, err := proxy.Kite.Register(proxyRegisterURL)
	if err != nil {
		t.Error(err)
	}

	// start now backend kite
	color.Green("Starting BackendKite")
	backendKite := kite.New("backendKite", "1.0.0")
	backendKite.Config = conf.Copy()
	backendKite.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})

	backendKite.Config.Port = 7777
	kiteUrl := &url.URL{Scheme: "http", Host: "localhost:7777", Path: "/kite"}

	go backendKite.Run()
	<-backendKite.ServerReadyNotify()

	// now search for a proxy from kontrol
	color.Green("BackendKite is searching proxy from kontrol")
	kites, err := backendKite.GetKites(protocol.KontrolQuery{
		Username:    "testuser",
		Environment: config.DefaultConfig.Environment,
		Name:        Name,
	})
	if err != nil {
		t.Fatal(err)
	}

	proxyKite := kites[0]
	err = proxyKite.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// backendKite is connected to proxy, now let us register to proxy and get
	// a proxy url. We send our url to proxy, it needs it in order to proxy us
	color.Green("Backendkite found proxy, now registering to it")
	result, err := proxyKite.TellWithTimeout("register", 4*time.Second, kiteUrl.String())
	if err != nil {
		t.Fatal(err)
	}

	proxyURL := result.MustString()
	if !strings.Contains(proxyURL, "/proxy") {
		t.Fatalf("Invalid proxy URL: %s", proxyURL)
	}

	registerURL, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}

	// register ourself to kontrol with this proxyUrl
	color.Green("BackendKite is registering to Kontrol with the result from proxy")
	go backendKite.RegisterForever(registerURL)
	<-backendKite.KontrolReadyNotify()

	// now another completely foreign kite and will search for our backend
	// kite, connect to it and execute the "foo" method
	color.Green("Foreign kite started")
	foreignKite := kite.New("foreignKite", "1.0.0")
	foreignKite.Config = conf.Copy()

	color.Green("Querying backendKite now")
	backendKites, err := foreignKite.GetKites(protocol.KontrolQuery{
		Username:    "testuser",
		Environment: config.DefaultConfig.Environment,
		Name:        "backendKite",
	})

	remoteBackendKite := backendKites[0]
	color.Green("Dialing BackendKite")
	err = remoteBackendKite.Dial()
	if err != nil {
		t.Fatal(err)
	}

	// foreignKite is connected to backendKite via proxy kite, fire our call...
	color.Green("Calling BackendKite's foo method")
	result, err = remoteBackendKite.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	s := result.MustString()
	if s != "bar" {
		t.Fatalf("Wrong reply: %s", s)
	}
}
