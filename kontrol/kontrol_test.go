package kontrol

import (
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
	uuid "github.com/satori/go.uuid"
)

var (
	conf *Config
	kon  *Kontrol
)

var interactive = os.Getenv("TEST_INTERACTIVE") == "1"

type Config struct {
	Config  *config.Config
	Private string
	Public  string
}

func startKontrol(pem, pub string, port int) (*Kontrol, *Config) {
	conf := config.New()
	conf.Username = "testuser"
	conf.KontrolURL = fmt.Sprintf("http://localhost:%d/kite", port)
	conf.KontrolKey = pub
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewToken("kontrol", pem, pub).Raw
	conf.Transport = config.XHRPolling
	conf.ReadEnvironmentVariables()

	DefaultPort = port
	kon := New(conf.Copy(), "1.0.0")

	switch os.Getenv("KONTROL_STORAGE") {
	case "etcd":
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	case "postgres":
		p := NewPostgres(nil, kon.Kite.Log)
		kon.SetStorage(p)
		kon.SetKeyPairStorage(p)
	default:
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	}

	kon.AddKeyPair("", pub, pem)

	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	return kon, &Config{
		Config:  conf,
		Private: pem,
		Public:  pub,
	}
}

type HelloKite struct {
	Kite  *kite.Kite
	URL   *url.URL
	Token bool

	clients map[string]*kite.Client
}

func NewHelloKite(name string, conf *Config) (*HelloKite, error) {
	k := kite.New(name, "1.0.0")
	k.Config = conf.Config.Copy()
	k.Config.Port = 0
	k.Config.KiteKey = testutil.NewToken(name, conf.Private, conf.Public).Raw
	k.SetLogLevel(kite.DEBUG)

	k.HandleFunc("hello", func(r *kite.Request) (interface{}, error) {
		return fmt.Sprintf("%s says hello", name), nil
	})

	go k.Run()
	<-k.ServerReadyNotify()

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", k.Port()),
		Path:   "/kite",
	}

	if err := k.RegisterForever(u); err != nil {
		k.Close()
		return nil, err
	}

	return &HelloKite{
		Kite:    k,
		URL:     u,
		clients: make(map[string]*kite.Client),
	}, nil
}

func (hk *HelloKite) Hello(remote *HelloKite) (string, error) {
	return hk.hello(remote, false)
}

func (hk *HelloKite) hello(remote *HelloKite, force bool) (string, error) {
	const timeout = 10 * time.Second

	c, ok := hk.clients[remote.Kite.Id]
	if !ok || force {
		if hk.Token {
			query := &protocol.KontrolQuery{
				ID: remote.Kite.Id,
			}

			kites, err := hk.Kite.GetKites(query)
			if err != nil {
				return "", err
			}

			if len(kites) != 1 {
				return "", fmt.Errorf("%s: expected to get 1 kite, got %d", remote.Kite.Id, len(kites))
			}

			c = kites[0]
		} else {
			c = hk.Kite.NewClient(remote.URL.String())
			c.Auth = &kite.Auth{
				Type: "kiteKey",
				Key:  hk.Kite.Config.KiteKey,
			}
		}

		if err := c.DialTimeout(timeout); err != nil {
			return "", nil
		}

		hk.clients[remote.Kite.Id] = c
	}

	res, err := c.TellWithTimeout("hello", timeout)
	if err != nil {
		// TODO(rjeczalik): remove "timeout" - see comment in (*Client).sendHub method
		if e, ok := err.(*kite.Error); ok && (e.Type == "disconnect" || e.Type == "timeout") && !force {
			return hk.hello(remote, true)
		}

		return "", err
	}

	s, err := res.String()
	if err != nil {
		return "", err
	}

	return s, nil
}

func (hk *HelloKite) Close() error {
	for _, c := range hk.clients {
		c.Close()
	}

	hk.Kite.Close()

	return nil
}

type HelloKites map[*HelloKite]*HelloKite

func Call(kitePairs HelloKites) error {
	for local, remote := range kitePairs {
		call := fmt.Sprintf("%s -> %s", local.Kite.Kite().Name, remote.Kite.Kite().Name)

		got, err := local.Hello(remote)
		if err != nil {
			return fmt.Errorf("%s: error calling: %s", call, err)
		}

		if want := fmt.Sprintf("%s says hello", remote.Kite.Kite().Name); got != want {
			return fmt.Errorf("%s: invalid response: got %q, want %q", call, got, want)
		}
	}

	return nil
}

func WaitTillConnected(conf *Config, timeout time.Duration, hks ...*HelloKite) error {
	k := kite.New("WaitTillConnected", "1.0.0")
	k.Config = conf.Config.Copy()

	t := time.After(timeout)

	for {
		select {
		case <-t:
			return fmt.Errorf("timed out waiting for %v after %s", hks, timeout)
		default:
			kites, err := k.GetKites(&protocol.KontrolQuery{
				Version: "1.0.0",
			})
			if err != nil && err != kite.ErrNoKitesAvailable {
				return err
			}

			notReady := make(map[string]struct{}, len(hks))
			for _, hk := range hks {
				notReady[hk.Kite.Kite().ID] = struct{}{}
			}

			for _, kite := range kites {
				delete(notReady, kite.Kite.ID)
			}

			if len(notReady) == 0 {
				return nil
			}
		}
	}
}

func pause(args ...interface{}) {
	if testing.Verbose() && interactive {
		fmt.Println("[PAUSE]", fmt.Sprint(args...), fmt.Sprintf(`("kill -1 %d" to continue)`, os.Getpid()))
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGHUP)
		<-ch
		signal.Stop(ch)
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	kon, conf = startKontrol(testkeys.Private, testkeys.Public, 5500)
}

func TestUpdateKeys(t *testing.T) {
	if storage := os.Getenv("KONTROL_STORAGE"); storage != "postgres" {
		t.Skip("skipping TestUpdateKeys for storage %q: not implemented", storage)
	}

	kon, conf := startKontrol(testkeys.Private, testkeys.Public, 5501)

	hk1, err := NewHelloKite("kite1", conf)
	if err != nil {
		t.Fatalf("error creating kite1: %s", err)
	}
	defer hk1.Close()

	hk2, err := NewHelloKite("kite2", conf)
	if err != nil {
		t.Fatalf("error creating kite1: %s", err)
	}
	defer hk2.Close()

	hk2.Token = true

	calls := HelloKites{
		hk1: hk2,
		hk2: hk1,
	}

	if err := Call(calls); err != nil {
		t.Fatal(err)
	}

	kon.Close()

	if err := kon.DeleteKeyPair("", testkeys.Public); err != nil {
		t.Fatalf("error deleting old pem: %s", err)
	}

	kon, conf = startKontrol(testkeys.PrivateSecond, testkeys.PublicSecond, 5501)

	pause("started kontrol 2")

	hk3, err := NewHelloKite("kite3", conf)
	if err != nil {
		t.Fatalf("error creating kite3: %s", err)
	}
	defer hk3.Close()

	if err := WaitTillConnected(conf, 15*time.Second, hk1, hk2, hk3); err != nil {
		t.Fatal(err)
	}

	calls = HelloKites{
		hk3: hk1,
		hk3: hk2,
		hk2: hk1,
		hk1: hk3,
		hk1: hk2,
	}

	if err := Call(calls); err != nil {
		t.Fatal(err)
	}

	pause("kite2 -> kite3 starting")

	// TODO(rjeczalik): enable after implementing kite key updates in kontrol
	//
	// if err := Call(HelloKites{hk2: hk3}); err != nil {
	//   t.Fatal(err)
	// }
}

func TestRegisterMachine(t *testing.T) {
	key, err := kon.registerUser("foo", testkeys.Public, testkeys.Private)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	token, err := jwt.Parse(key, kitekey.GetKontrolKey)
	if err != nil {
		t.Fatalf(err.Error())
	}

	if username := token.Claims["sub"].(string); username != "foo" {
		t.Fatalf("invalid username: %s", username)
	}
}

func TestTokenInvalidation(t *testing.T) {
	oldval := TokenTTL
	defer func() {
		TokenTTL = oldval
	}()

	TokenTTL = time.Millisecond * 500
	TokenLeeway = 0

	testName := "mathworker6"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Config.Copy()
	m.Config.Port = 6666

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:6666", Path: "/mathworker6"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	token, err := m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}

	time.Sleep(time.Millisecond * 700)

	token2, err := m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}

	if token == token2 {
		t.Error("token invalidation doesn't work")
	}

	TokenTTL = time.Second * 4

	token3, err := m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}

	token4, err := m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}

	if token3 != token4 {
		t.Error("tokens should be the same")
	}
}

func TestMultiple(t *testing.T) {
	testDuration := time.Second * 10

	// number of kites that will be queried. Means if there are 50 example
	// kites available only 10 of them will be queried. Increasing this number
	// makes the test fail.
	kiteNumber := 5

	// number of clients that will query example kites
	clientNumber := 10

	for i := 0; i < kiteNumber; i++ {
		m := kite.New("example"+strconv.Itoa(i), "0.1."+strconv.Itoa(i))
		m.Config = conf.Config.Copy()

		kiteURL := &url.URL{Scheme: "http", Host: "localhost:4444", Path: "/kite"}
		err := m.RegisterForever(kiteURL)
		if err != nil {
			t.Error(err)
		}
		defer m.Close()
	}

	clients := make([]*kite.Kite, clientNumber)
	for i := 0; i < clientNumber; i++ {
		c := kite.New("client"+strconv.Itoa(i), "0.0.1")
		c.Config = conf.Config.Copy()
		c.SetupKontrolClient()
		clients[i] = c
	}

	var wg sync.WaitGroup

	timeout := time.After(testDuration)

	// every one second
	for {
		select {
		case <-time.Tick(time.Second):
			for i := 0; i < clientNumber; i++ {
				wg.Add(1)

				go func(i int) {
					defer wg.Done()

					time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)))

					query := &protocol.KontrolQuery{
						Username:    conf.Config.Username,
						Environment: conf.Config.Environment,
						Name:        "example" + strconv.Itoa(rand.Intn(kiteNumber)),
					}

					start := time.Now()
					_, err := clients[i].GetKites(query)
					elapsedTime := time.Since(start)
					if err != nil {
						// we don't fail here otherwise pprof can't gather information
						fmt.Printf("[%d] aborted, elapsed %f sec err: %s\n",
							i, elapsedTime.Seconds(), err)
					} else {
						// fmt.Printf("[%d] finished, elapsed %f sec\n", i, elapsedTime.Seconds())
					}
				}(i)
			}
		case <-timeout:
			t.SkipNow()
		}

	}

	wg.Wait()
}

func TestGetKites(t *testing.T) {
	testName := "mathworker4"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Config.Copy()

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:4444", Path: "/kite"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	query := &protocol.KontrolQuery{
		Username:    conf.Config.Username,
		Environment: conf.Config.Environment,
		Name:        testName,
		Version:     "1.1.1",
	}

	// exp2 queries for mathkite
	exp3 := kite.New("exp3", "0.0.1")
	exp3.Config = conf.Config.Copy()
	kites, err := exp3.GetKites(query)
	if err != nil {
		t.Fatal(err)
	}

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	if len(kites) != 1 {
		t.Fatalf("Only one kite is registerd, we have %d", len(kites))
	}

	if kites[0].Name != testName {
		t.Errorf("getkites got %s exptected %", kites[0].Name, testName)
	}

	if kites[0].Version != testVersion {
		t.Errorf("getkites got %s exptected %", kites[0].Version, testVersion)
	}
}

func TestGetToken(t *testing.T) {
	testName := "mathworker5"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Config.Copy()
	m.Config.Port = 6666

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:6666", Path: "/kite"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	_, err = m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}
}

func TestRegisterKite(t *testing.T) {
	kiteURL := &url.URL{Scheme: "http", Host: "localhost:4444", Path: "/kite"}
	m := kite.New("mathworker3", "1.1.1")
	m.Config = conf.Config.Copy()

	res, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	if kiteURL.String() != res.URL.String() {
		t.Error("register: got %s expected %s", res.URL.String(), kiteURL.String())
	}
}

func TestKontrol(t *testing.T) {
	// Start mathworker
	mathKite := kite.New("mathworker", "1.2.3")
	mathKite.Config = conf.Config.Copy()
	mathKite.Config.Port = 6161
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp2 kite is the mathworker client
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Config.Copy()

	query := &protocol.KontrolQuery{
		Username:    exp2Kite.Kite().Username,
		Environment: exp2Kite.Kite().Environment,
		Name:        "mathworker",
		Version:     "~> 1.1",
	}

	// exp2 queries for mathkite
	kites, err := exp2Kite.GetKites(query)
	if err != nil {
		t.Fatal(err)
	}

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	// exp2 connectes to mathworker
	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Fatal("Cannot connect to remote mathworker", err)
	}

	// Test Kontrol.GetToken
	tokenCache = make(map[string]string) // empty it
	_, err = exp2Kite.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Error(err)
	}

	// Run "square" method
	response, err := remoteMathWorker.TellWithTimeout("square", 4*time.Second, 2)
	if err != nil {
		t.Fatal(err)
	}

	var result int
	err = response.Unmarshal(&result)
	if err != nil {
		t.Fatal(err)
	}

	// Result must be "4"
	if result != 4 {
		t.Fatalf("Invalid result: %d", result)
	}

}

func Square(r *kite.Request) (interface{}, error) {
	a, err := r.Args.One().Float64()
	if err != nil {
		return nil, err
	}

	result := a * a
	return result, nil
}

func TestGetQueryKey(t *testing.T) {
	// This query is valid because there are no gaps between query fields.
	q := &protocol.KontrolQuery{
		Username:    "cenk",
		Environment: "production",
	}
	key, err := GetQueryKey(q)
	if err != nil {
		t.Errorf(err.Error())
	}
	if key != "/cenk/production" {
		t.Errorf("Unexpected key: %s", key)
	}

	// This is wrong because Environment field is empty.
	// We can't make a query on etcd because wildcards are not allowed in paths.
	q = &protocol.KontrolQuery{
		Username: "cenk",
		Name:     "fs",
	}
	key, err = GetQueryKey(q)
	if err == nil {
		t.Errorf("Error is expected")
	}
	if key != "" {
		t.Errorf("Key is not expected: %s", key)
	}

	// This is also wrong becaus each query must have a non-empty username field.
	q = &protocol.KontrolQuery{
		Environment: "production",
		Name:        "fs",
	}
	key, err = GetQueryKey(q)
	if err == nil {
		t.Errorf("Error is expected")
	}
	if key != "" {
		t.Errorf("Key is not expected: %s", key)
	}
}

func TestKontrolMultiKey(t *testing.T) {
	i := uuid.NewV4()
	secondID := i.String()
	// add so we can use it as key
	if err := kon.AddKeyPair(secondID, testkeys.PublicSecond, testkeys.PrivateSecond); err != nil {
		t.Fatal(err)
	}

	// Start mathworker
	mathKite := kite.New("mathworker2", "2.0.0")
	mathKite.Config = conf.Config.Copy()
	mathKite.Config.Port = 6162
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp3 kite is the mathworker client. However it uses a different public
	// key
	exp3Kite := kite.New("exp3", "0.0.1")
	exp3Kite.Config = conf.Config.Copy()
	exp3Kite.Config.KiteKey = testutil.NewKiteKeyWithKeyPair(testkeys.PrivateSecond, testkeys.PublicSecond).Raw
	exp3Kite.Config.KontrolKey = testkeys.PublicSecond

	query := &protocol.KontrolQuery{
		Username:    exp3Kite.Kite().Username,
		Environment: exp3Kite.Kite().Environment,
		Name:        "mathworker2",
		Version:     "2.0.0",
	}

	// exp3 queries for mathkite
	kites, err := exp3Kite.GetKites(query)
	if err != nil {
		t.Fatal(err)
	}

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	// exp3 connectes to mathworker
	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Fatal("Cannot connect to remote mathworker", err)
	}

	// Test Kontrol.GetToken
	tokenCache = make(map[string]string) // empty it
	newToken, err := exp3Kite.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Error(err)
	}

	if remoteMathWorker.Auth.Key == newToken {
		t.Errorf("Token renew failed. Tokens should be different after renew")
	}

	// Run "square" method
	response, err := remoteMathWorker.TellWithTimeout("square", 4*time.Second, 2)
	if err != nil {
		t.Fatal(err)
	}

	var result int
	err = response.Unmarshal(&result)
	if err != nil {
		t.Fatal(err)
	}

	// Result must be "4"
	if result != 4 {
		t.Fatalf("Invalid result: %d", result)
	}

	// now invalidate the second key
	log.Printf("Invalidating %s\n", secondID)
	if err := kon.DeleteKeyPair(secondID, ""); err != nil {
		t.Fatal(err)
	}

	// try to get a new key, this should replace exp3Kite.Config.KontrolKey
	// with the new (in our case because PublicSecond is invalidated, it's
	// going to use Public (the first key). Also it should return the new Key.
	publicKey, err := exp3Kite.GetKey()
	if err != nil {
		t.Fatal(err)
	}

	if publicKey != testkeys.Public {
		t.Errorf("Key renew failed\n\twant:%s\n\tgot :%s\n", testkeys.Public, publicKey)
	}

	if exp3Kite.Config.KontrolKey != publicKey {
		t.Errorf("Key renew should replace config.KontrolKey\n\twant:%s\n\tgot :%s\n",
			testkeys.Public, publicKey)
	}
}
