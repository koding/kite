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
	conf *config.Config
	kon  *Kontrol
)

var interactive = os.Getenv("TEST_INTERACTIVE") == "1"

func startKontrol(pem, pub string, port int) (kon *Kontrol, conf *config.Config) {
	conf = config.New()
	conf.Username = "testuser"
	conf.KontrolURL = fmt.Sprintf("http://localhost:%d/kite", port)
	conf.KontrolKey = pub
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKeyWithKeyPair(pem, pub).Raw
	conf.ReadEnvironmentVariables()

	DefaultPort = port
	kon = New(conf.Copy(), "0.0.1")

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

	return kon, conf
}

func helloKite(name string, conf *config.Config) (*kite.Kite, *url.URL, error) {
	k := kite.New(name, "1.0.0")
	k.Config = conf.Copy()
	k.Config.Port = 0

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
		return nil, nil, err
	}

	return k, u, nil
}

func hello(k *kite.Kite, u *url.URL) (string, error) {
	const timeout = 10 * time.Second

	c := k.NewClient(u.String())
	c.Auth = &kite.Auth{
		Type: "kiteKey",
		Key:  k.Config.KiteKey,
	}

	if err := c.DialTimeout(timeout); err != nil {
		return "", nil
	}
	defer c.Close()

	res, err := c.TellWithTimeout("hello", timeout)
	if err != nil {
		return "", err
	}

	s, err := res.String()
	if err != nil {
		return "", err
	}

	return s, nil
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
	pause("starting TestUpdateKeys")

	kon, conf := startKontrol(testkeys.Private, testkeys.Public, 5501)
	defer kon.Close()

	pause("kontrol 1 started")

	k1, u1, err := helloKite("kite1", conf)
	if err != nil {
		t.Fatalf("error creating kite1: %s", err)
	}
	defer k1.Close()

	pause("kite 1 started")

	k2, u2, err := helloKite("kite2", conf)
	if err != nil {
		t.Fatalf("error creating kite1: %s", err)
	}
	defer k2.Close()

	pause("kite 2 started")

	msg, err := hello(k1, u2)
	if err != nil {
		t.Fatalf("error calling hello(kite1 -> kite2): %s", err)
	}

	if want := "kite2 says hello"; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}

	msg, err = hello(k2, u1)
	if err != nil {
		t.Fatalf("error calling hello(kite2 -> kite1): %s", err)
	}

	if want := "kite1 says hello"; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}

	kon.Close()

	kon, conf = startKontrol(testkeys.PrivateSecond, testkeys.PublicSecond, 5501)

	pause("kontrol 2 started")

	k3, u3, err := helloKite("kite3", conf)
	if err != nil {
		t.Fatalf("error creating kite3: %s", err)
	}
	defer k1.Close()

	pause("kite 3 started")

	msg, err = hello(k3, u1)
	if err != nil {
		t.Fatalf("error calling hello(kite3 -> kite1): %s", err)
	}

	if want := "kite1 says hello"; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}

	msg, err = hello(k1, u3)
	if err != nil {
		t.Fatalf("error calling hello(kite1 -> kite3): %s", err)
	}

	if want := "kite3 says hello"; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}
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
	m.Config = conf.Copy()
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
		m.Config = conf.Copy()

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
		c.Config = conf.Copy()
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
						Username:    conf.Username,
						Environment: conf.Environment,
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
	m.Config = conf.Copy()

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:4444", Path: "/kite"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	query := &protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        testName,
		Version:     "1.1.1",
	}

	// exp2 queries for mathkite
	exp3 := kite.New("exp3", "0.0.1")
	exp3.Config = conf.Copy()
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
	m.Config = conf.Copy()
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
	m.Config = conf.Copy()

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
	mathKite.Config = conf.Copy()
	mathKite.Config.Port = 6161
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp2 kite is the mathworker client
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Copy()

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
	mathKite.Config = conf.Copy()
	mathKite.Config.Port = 6162
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp3 kite is the mathworker client. However it uses a different public
	// key
	exp3Kite := kite.New("exp3", "0.0.1")
	exp3Kite.Config = conf.Copy()
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
