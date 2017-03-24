package kontrol

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
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

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	kon, conf = startKontrol(testkeys.Private, testkeys.Public, 5500)
}

func TestUpdateKeys(t *testing.T) {
	if storage := os.Getenv("KONTROL_STORAGE"); storage != "postgres" {
		t.Skip("skipping TestUpdateKeys for storage %q: not implemented", storage)
	}

	kon, conf := startKontrol(testkeys.PrivateThird, testkeys.PublicThird, 5501)

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

	if err := kon.DeleteKeyPair("", testkeys.PublicThird); err != nil {
		t.Fatalf("error deleting key pair: %s", err)
	}

	kon, conf = startKontrol(testkeys.Private, testkeys.Public, 5501)
	defer kon.Close()

	reg, err := hk1.WaitRegister(15 * time.Second)
	if err != nil {
		t.Fatalf("kite1 register error: %s", err)
	}

	if reg.PublicKey != testkeys.Public {
		t.Fatalf("kite1: got public key %q, want %q", reg.PublicKey, testkeys.Public)
	}

	if reg.KiteKey == "" {
		t.Fatal("kite1: kite key was not updated")
	}

	reg, err = hk2.WaitRegister(15 * time.Second)
	if err != nil {
		t.Fatalf("kite2 register error: %s", err)
	}

	if reg.PublicKey != testkeys.Public {
		t.Fatalf("kite2: got public key %q, want %q", reg.PublicKey, testkeys.Public)
	}

	if reg.KiteKey == "" {
		t.Fatal("kite2: kite key was not updated")
	}

	pause("token renew")

	calls = HelloKites{
		hk1: hk2,
		hk2: hk1,
	}

	if err := Call(calls); err == nil || !strings.Contains(err.Error(), "token is expired") {
		t.Fatalf("got nil or unexpected error: %v", err)
	}

	if err := hk2.WaitTokenExpired(10 * time.Second); err != nil {
		t.Fatalf("kite2: waiting for token expire: %s", err)
	}

	if err := hk2.WaitTokenRenew(10 * time.Second); err != nil {
		t.Fatalf("kite2: waiting for token renew: %s", err)
	}

	calls = HelloKites{
		hk1: hk2,
		hk2: hk1,
	}

	if err := Call(calls); err != nil {
		t.Fatal(err)
	}

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

	if err := Call(HelloKites{hk2: hk3}); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterMachine(t *testing.T) {
	key, err := kon.registerUser("foo", testkeys.Public, testkeys.Private)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	claims := &kitekey.KiteClaims{}

	_, err = jwt.ParseWithClaims(key, claims, kitekey.GetKontrolKey)
	if err != nil {
		t.Fatalf(err.Error())
	}

	if claims.Subject != "foo" {
		t.Fatalf("invalid username: %s", claims.Subject)
	}
}

func TestRegisterDenyEvil(t *testing.T) {
	// TODO(rjeczalik): use sentinel error value instead
	const authErr = "no valid authentication key found"

	legit, err := NewHelloKite("testuser", conf)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = legit.Kite.GetToken(legit.Kite.Kite()); err != nil {
		t.Fatal(err)
	}

	evil := kite.New("testuser", "1.0.0")
	evil.Config = conf.Config.Copy()
	evil.Config.Port = 6767
	evil.Config.Username = "testuser"
	evil.Config.KontrolUser = "testuser"
	evil.Config.KontrolURL = conf.Config.KontrolURL
	evil.Config.KiteKey = testutil.NewToken("testuser", testkeys.PrivateEvil, testkeys.PublicEvil).Raw
	// KontrolKey can be easily extracted from existing kite.key
	evil.Config.KontrolKey = testkeys.Public
	evil.Config.ReadEnvironmentVariables()

	evilURL := &url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:6767",
		Path:   "/kite",
	}

	_, err = evil.Register(evilURL)
	if err == nil {
		t.Errorf("expected kontrol to deny register request: %s", evil.Kite())
	} else {
		t.Logf("register denied: %s", err)
	}

	_, err = evil.GetToken(legit.Kite.Kite())
	if err == nil {
		t.Errorf("expected kontrol to deny token request: %s", evil.Kite())
	} else {
		t.Logf("token denied: %s", err)
	}

	_, err = evil.TellKontrolWithTimeout("registerMachine", 4*time.Second, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected registerMachine to fail")
	}
	if !strings.Contains(err.Error(), authErr) {
		t.Fatalf("got %q, want %q error", err, authErr)
	}
}

func TestMultipleRegister(t *testing.T) {
	confCopy := *conf
	confCopy.RegisterFunc = func(hk *HelloKite) error {
		hk.Kite.RegisterHTTPForever(hk.URL)

		if _, err := hk.WaitRegister(15 * time.Second); err != nil {
			hk.Kite.Close()
			return err
		}

		return nil
	}

	hk, err := NewHelloKite("kite", &confCopy)
	if err != nil {
		t.Fatalf("error creating kite: %s", err)
	}
	defer hk.Close()

	query := &protocol.KontrolQuery{
		ID: hk.Kite.Kite().ID,
	}

	c, err := hk.Kite.GetKites(query)
	if err != nil {
		t.Fatalf("GetKites()=%s", err)
	}
	klose(c)

	if len(c) != 1 {
		t.Fatalf("want len(c) = 1; got %d", len(c))
	}

	if c[0].URL != hk.URL.String() {
		t.Fatalf("want url = %q; got %q", hk.URL, c[0].URL)
	}

	urlCopy := *hk.URL
	_, port, err := net.SplitHostPort(hk.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	urlCopy.Host = net.JoinHostPort("localhost", port)

	if _, err := hk.Kite.RegisterHTTP(&urlCopy); err != nil {
		t.Fatal(err)
	}

	if _, err := hk.WaitRegister(15 * time.Second); err != nil {
		t.Fatal(err)
	}

	timeout := time.After(2 * time.Minute)

	query = &protocol.KontrolQuery{
		ID: hk.Kite.Kite().ID,
	}

	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for RegisterURL to update")
		default:
			c, err := hk.Kite.GetKites(query)
			if err != nil {
				t.Fatalf("GetKites()=%s", err)
			}
			klose(c)

			if len(c) != 1 {
				t.Fatalf("want len(c) = 1; got %d", len(c))
			}

			if c[0].URL == urlCopy.String() {
				return
			}

			time.Sleep(10 * time.Second)
		}
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
	defer m.Close()

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:6666", Path: "/mathworker6"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}

	oldToken, err := m.GetToken(m.Kite())
	if err != nil {
		t.Error(err)
	}

	token, err := m.GetTokenForce(m.Kite())
	if err != nil {
		t.Error(err)
	}

	if oldToken == token {
		t.Errorf("want %q != %q", oldToken, token)
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
	kiteNumber := runtime.GOMAXPROCS(0)

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
		defer c.Close()
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
					k, err := clients[i].GetKites(query)
					elapsedTime := time.Since(start)
					if err != nil {
						// we don't fail here otherwise pprof can't gather information
						fmt.Printf("[%d] aborted, elapsed %f sec err: %s\n",
							i, elapsedTime.Seconds(), err)
					} else {
						klose(k)
						// fmt.Printf("[%d] finished, elapsed %f sec\n", i, elapsedTime.Seconds())
					}
				}(i)

				wg.Wait()
			}
		case <-timeout:
			return
		}
	}
}

func TestGetKites(t *testing.T) {
	testName := "mathworker4"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Config.Copy()
	defer m.Close()

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
	defer exp3.Close()
	kites, err := exp3.GetKites(query)
	if err != nil {
		t.Fatal(err)
	}
	defer klose(kites)

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	if len(kites) != 1 {
		t.Fatalf("Only one kite is registered, we have %d", len(kites))
	}

	if kites[0].Name != testName {
		t.Errorf("getkites got %s exptected %s", kites[0].Name, testName)
	}

	if kites[0].Version != testVersion {
		t.Errorf("getkites got %s exptected %s", kites[0].Version, testVersion)
	}
}

func TestGetToken(t *testing.T) {
	testName := "mathworker5"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Config.Copy()
	m.Config.Port = 6666
	defer m.Close()

	kiteURL := &url.URL{Scheme: "http", Host: "localhost:6666", Path: "/kite"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}

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
		t.Fatal(err)
	}
	defer m.Close()

	if kiteURL.String() != res.URL.String() {
		t.Errorf("register: got %s expected %s", res.URL.String(), kiteURL.String())
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
	defer mathKite.Close()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp2 kite is the mathworker client
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Config.Copy()
	defer exp2Kite.Close()

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
	defer klose(kites)

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	// exp2 connects to mathworker
	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Fatal("Cannot connect to remote mathworker", err)
	}

	// Test Kontrol.GetToken
	// TODO(rjeczalik): rework test to not touch Kontrol internals
	kon.tokenCacheMu.Lock()
	kon.tokenCache = make(map[string]cachedToken)
	kon.tokenCacheMu.Unlock()

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
	if storage := os.Getenv("KONTROL_STORAGE"); storage != "postgres" {
		t.Skip("%q storage does not currently implement soft key pair deletes", storage)
	}

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
	defer mathKite.Close()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp3 kite is the mathworker client. However it uses a different public
	// key
	exp3Kite := kite.New("exp3", "0.0.1")
	exp3Kite.Config = conf.Config.Copy()
	exp3Kite.Config.KiteKey = testutil.NewKiteKeyWithKeyPair(testkeys.PrivateSecond, testkeys.PublicSecond).Raw
	exp3Kite.Config.KontrolKey = testkeys.PublicSecond
	defer exp3Kite.Close()

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
	defer klose(kites)

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	// exp3 connects to mathworker
	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Fatal("Cannot connect to remote mathworker", err)
	}

	// Test Kontrol.GetToken
	// TODO(rjeczalik): rework test to not touch Kontrol internals
	kon.tokenCacheMu.Lock()
	kon.tokenCache = make(map[string]cachedToken) // empty it
	kon.tokenCacheMu.Unlock()

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
