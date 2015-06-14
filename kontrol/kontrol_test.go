package kontrol

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

var (
	conf *config.Config
	kon  *Kontrol
)

func init() {
	conf = config.New()
	conf.Username = "testuser"
	conf.KontrolURL = "http://localhost:5555/kite"
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw
	conf.ReadEnvironmentVariables()

	DefaultPort = 5555
	kon = New(conf.Copy(), "0.0.1")

	switch os.Getenv("KONTROL_STORAGE") {
	case "etcd":
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	case "postgres":
		p := NewPostgres(nil, kon.Kite.Log)
		kon.SetStorage(p)
		kon.SetKeyPairStorage(&PostgresMockKeyPair{
			Postgres: p,
		})
	default:
		kon.SetStorage(NewEtcd(nil, kon.Kite.Log))
	}

	kon.AddKeyPair("", testkeys.Public, testkeys.Private)

	go kon.Run()
	<-kon.Kite.ServerReadyNotify()

	rand.Seed(time.Now().UTC().UnixNano())
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

	t.Log("Setting up mathworker6")
	testName := "mathworker6"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Copy()
	m.Config.Port = 6666

	t.Log("Registering mathworker6")
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

	fmt.Printf("Creating %d example kites\n", kiteNumber)
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

	fmt.Printf("Creating %d clients\n", clientNumber)
	clients := make([]*kite.Kite, clientNumber)
	for i := 0; i < clientNumber; i++ {
		c := kite.New("client"+strconv.Itoa(i), "0.0.1")
		c.Config = conf.Copy()
		c.SetupKontrolClient()
		clients[i] = c
	}

	var wg sync.WaitGroup

	fmt.Printf("Querying for example kites with %d conccurent clients randomly\n", clientNumber)
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
			fmt.Println("test stopped")
			t.SkipNow()
		}

	}

	wg.Wait()
}

func TestGetKites(t *testing.T) {
	t.Log("Setting up mathworker4")

	testName := "mathworker4"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Copy()

	t.Log("Registering ", testName)
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
	t.Log("Querying for mathworker4")
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
	t.Log("Setting up mathworker5")
	testName := "mathworker5"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Copy()
	m.Config.Port = 6666

	t.Log("Registering mathworker5")
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
	t.Log("Setting up mathworker3")
	kiteURL := &url.URL{Scheme: "http", Host: "localhost:4444", Path: "/kite"}
	m := kite.New("mathworker3", "1.1.1")
	m.Config = conf.Copy()

	t.Log("Registering mathworker")
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
	t.Log("Setting up mathworker")
	mathKite := kite.New("mathworker", "1.2.3")
	mathKite.Config = conf.Copy()
	mathKite.Config.Port = 6161
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp2 kite is the mathworker client
	t.Log("Setting up exp2 kite")
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Copy()

	query := &protocol.KontrolQuery{
		Username:    exp2Kite.Kite().Username,
		Environment: exp2Kite.Kite().Environment,
		Name:        "mathworker",
		Version:     "~> 1.1",
	}

	// exp2 queries for mathkite
	t.Log("Querying for mathworkers")
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
	t.Logf("oldToken: %s", remoteMathWorker.Auth.Key)
	newToken, err := exp2Kite.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Error(err)
	}
	t.Logf("newToken: %s", newToken)

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
	// add so we can use it as key
	if err := kon.AddKeyPair("", testkeys.PublicSecond, testkeys.PrivateSecond); err != nil {
		t.Fatal(err)
	}

	// Start mathworker
	t.Log("Setting up mathworker")
	mathKite := kite.New("mathworker", "1.2.3")
	mathKite.Config = conf.Copy()
	mathKite.Config.Port = 6162
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// exp2 kite is the mathworker client. However it uses a different public
	// key
	t.Log("Setting up exp2 kite")
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Copy()
	exp2Kite.Config.KiteKey = testutil.NewKiteKeyWithKeyPair(testkeys.PrivateSecond, testkeys.PublicSecond).Raw
	exp2Kite.Config.KontrolKey = testkeys.PrivateSecond

	query := &protocol.KontrolQuery{
		Username:    exp2Kite.Kite().Username,
		Environment: exp2Kite.Kite().Environment,
		Name:        "mathworker",
		Version:     "~> 1.1",
	}

	// exp2 queries for mathkite
	t.Log("Querying for mathworkers")
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
	t.Logf("oldToken: %s", remoteMathWorker.Auth.Key)
	newToken, err := exp2Kite.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Error(err)
	}
	t.Logf("newToken: %s", newToken)

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

func TestKeyRenew(t *testing.T) {
	// This key will be used as key replacement
	kon.AddKeyPair("", testkeys.PublicSecond, testkeys.PrivateSecond)

	// This kite is using the old key. We are going to invalidate it and thus a
	// new key will be used
	t.Log("Setting up mathworker")
	mathKite := kite.New("mathworker", "1.2.3")
	mathKite.Config = conf.Copy()
	mathKite.Config.Port = 6163
	mathKite.HandleFunc("square", Square)
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()
	go mathKite.RegisterForever(&url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(mathKite.Config.Port), Path: "/kite"})
	<-mathKite.KontrolReadyNotify()

	// get the key we want to delete
	keyPair, err := kon.keyPair.GetKeyFromPublic(testkeys.Public)
	if err != nil {
		t.Fatal(err)
	}

	// now remove the old Key
	if err := kon.keyPair.DeleteKey(&KeyPair{
		ID: keyPair.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// try to get a new key, this should replace mathKite.Config.KontrolKey
	// with the new key and also should return the new key
	publicKey, err := mathKite.GetKey()
	if err != nil {
		t.Fatal(err)
	}

	if publicKey != testkeys.PublicSecond {
		t.Errorf("Key renew failed\n\twant:%s\n\tgot :%s\n", testkeys.PublicSecond, publicKey)
	}

	if mathKite.Config.KontrolKey != publicKey {
		t.Errorf("Key renew should replace config.KontrolKey\n\twant:%s\n\tgot :%s\n",
			testkeys.PublicSecond, publicKey)
	}
}

// A PostgreMockKeyPair which deletes the key instead of updating the
// deleted_at row.
type PostgresMockKeyPair struct {
	*Postgres
}

func (p *PostgresMockKeyPair) DeleteKey(keyPair *KeyPair) error {
	// we need to first delete kites that depend on the key because of the
	// constraints we set up
	deleteKites := `DELETE FROM kite.kite WHERE key_id = $1`
	_, err := p.DB.Exec(deleteKites, keyPair.ID)

	deleteKey := `DELETE FROM kite.key WHERE id = $1`
	_, err = p.DB.Exec(deleteKey, keyPair.ID)
	return err
}
