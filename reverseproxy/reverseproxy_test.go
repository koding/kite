package reverseproxy

import (
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

func TestWebSocketProxy(t *testing.T) {
	color.Blue("====> Starting WebSocket test")
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:5555/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw
	conf.ReadEnvironmentVariables()

	// start kontrol
	color.Green("Starting kontrol")
	kontrol.DefaultPort = 5555
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

	// start proxy
	proxyRegistered := make(chan struct{})

	color.Green("Starting Proxy and registering to Kontrol")
	proxyConf := conf.Copy()
	proxyConf.Port = 4999
	proxy := New(proxyConf)
	proxy.PublicHost = "localhost"
	proxy.PublicPort = proxyConf.Port
	proxy.Scheme = "http"
	proxy.Kite.OnRegister(func(*protocol.RegisterResult) { close(proxyRegistered) })

	go proxy.Run()

	select {
	case <-proxy.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for proxy to start up")
	}

	proxyRegisterURL := &url.URL{
		Scheme: proxy.Scheme,
		Host:   proxy.PublicHost + ":" + strconv.Itoa(proxy.PublicPort),
		Path:   "/kite",
	}

	_, err := proxy.Kite.Register(proxyRegisterURL)
	if err != nil {
		t.Error(err)
	}

	select {
	case <-proxyRegistered:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for proxy to register")
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

	query := &protocol.KontrolQuery{
		Username:    "testuser",
		Environment: config.DefaultConfig.Environment,
		Name:        Name,
	}

	// now search for a proxy from kontrol
	color.Green("BackendKite is searching proxy from kontrol")
	kites, err := backendKite.GetKites(query)
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
	backendKites, err := foreignKite.GetKites(&protocol.KontrolQuery{
		Username:    "testuser",
		Environment: config.DefaultConfig.Environment,
		Name:        "backendKite",
	})

	if err != nil {
		t.Fatal(err)
	}

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
