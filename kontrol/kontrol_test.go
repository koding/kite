package kontrol

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/registration"
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
	conf.KontrolURL = &url.URL{Scheme: "ws", Host: "localhost:4000"}
	conf.KontrolKey = testkeys.Public
	conf.KontrolUser = "testuser"
	conf.KiteKey = testutil.NewKiteKey().Raw

	kon = New(conf.Copy(), "0.0.1", testkeys.Public, testkeys.Private)
	kon.DataDir, _ = ioutil.TempDir("", "")
	defer os.RemoveAll(kon.DataDir)
	kon.Start()
}

func BenchmarkGetKites(b *testing.B) {
	prepareKite := func(name, version string) func() {
		m := kite.New(name, version)
		m.Config = conf.Copy()

		kiteURL := &url.URL{Scheme: "ws", Host: "localhost:4444"}
		_, err := m.Register(kiteURL)
		if err != nil {
			b.Error(err)
		}

		return func() { m.Close() }
	}

	for i := 0; i < 100; i++ {
		cls := prepareKite("example", "0.1."+strconv.Itoa(i))
		defer cls() // close them later
	}

	query := protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        "example",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exp := kite.New("exp"+strconv.Itoa(i), "0.0.1")
		exp.Config = conf.Copy()
		_, err := exp.GetKites(query)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestMultiple(t *testing.T) {
	prepareKite := func(name, version string) func() {
		m := kite.New(name, version)
		m.Config = conf.Copy()

		kiteURL := &url.URL{Scheme: "ws", Host: "localhost:4444"}
		_, err := m.Register(kiteURL)
		if err != nil {
			t.Error(err)
		}

		return func() { m.Close() }
	}

	fmt.Println("Creating 100 example kites")
	for i := 0; i < 100; i++ {
		// cls := prepareKite("example"+strconv.Itoa(i), "0.1."+strconv.Itoa(i))
		cls := prepareKite("example", "0.1."+strconv.Itoa(i))
		defer cls() // close them later
	}

	n := 100 // increasing the number makes the test fail
	fmt.Printf("Querying for example kites with %d conccurent clients\n", n)

	// setup up clients earlier
	clients := make([]*kite.Kite, n)
	for i := 0; i < n; i++ {
		exp := kite.New("exp"+strconv.Itoa(i), "0.0.1")
		exp.Config = conf.Copy()
		// exp.Config.Username = conf.Username + strconv.Itoa(i)
		clients[i] = exp
	}

	query := protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        "example",
	}

	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := time.Now()

			_, err := clients[i].GetKites(query)

			elapsedTime := time.Since(start)
			end := time.Now()

			if err != nil {
				t.Errorf("[%d] aborted at %s (elapsed %f sec) err: %s\n",
					i, end.Format(time.StampMilli), elapsedTime.Seconds(), err)
			} else {
				fmt.Printf("[%d] finished at %s (elapsed %f sec)\n",
					i, end.Format(time.StampMilli), elapsedTime.Seconds())

			}
		}(i)
	}

	wg.Wait()
}

func TestGetKites(t *testing.T) {
	t.Log("Setting up mathworker4")

	testName := "mathwork4"
	testVersion := "1.1.1"
	m := kite.New(testName, testVersion)
	m.Config = conf.Copy()

	t.Log("Registering ", testName)
	kiteURL := &url.URL{Scheme: "ws", Host: "localhost:4444"}
	_, err := m.Register(kiteURL)
	if err != nil {
		t.Error(err)
	}
	defer m.Close()

	query := protocol.KontrolQuery{
		Username:    conf.Username,
		Environment: conf.Environment,
		Name:        testName,
		Version:     "~> 1.1",
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
		t.Fatal("Only one kite is registerd, we have %d", len(kites))
	}

	if kites[0].Name != testName {
		t.Error("getkites got %s exptected %", kites[0].Name, testName)
	}

	if kites[0].Version != testVersion {
		t.Error("getkites got %s exptected %", kites[0].Version, testVersion)
	}
}

func TestRegister(t *testing.T) {
	t.Log("Setting up mathworker3")
	kiteURL := &url.URL{Scheme: "ws", Host: "localhost:4444"}
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
	t.Log("Setting up proxy")
	prx := proxy.New(conf.Copy(), "0.0.1", testkeys.Public, testkeys.Private)
	prx.Start()

	time.Sleep(1e9)

	// Start mathworker
	t.Log("Setting up mathworker")
	mathKite := kite.New("mathworker", "1.2.3")
	mathKite.Config = conf.Copy()
	mathKite.HandleFunc("square", Square)
	mathKite.Start()

	reg := registration.New(mathKite)
	go reg.RegisterToProxyAndKontrol()
	<-reg.ReadyNotify()

	// exp2 kite is the mathworker client
	t.Log("Setting up exp2 kite")
	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Copy()

	query := protocol.KontrolQuery{
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
	t.Logf("oldToken: %s", remoteMathWorker.Authentication.Key)
	newToken, err := exp2Kite.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Error(err)
	}
	t.Logf("newToken: %s", newToken)

	// Run "square" method
	response, err := remoteMathWorker.Tell("square", 2)
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

	events := make(chan *kite.Event, 3)

	// Test WatchKites
	t.Log("calling  watchkites")
	watcher, err := exp2Kite.WatchKites(query, func(e *kite.Event, err *kite.Error) {
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("Event.Action: %s Event.Kite.ID: %s", e.Action, e.Kite.ID)
		events <- e
	})
	if err != nil {
		t.Fatalf("Cannot watch: %s", err.Error())
	}

	// First event must be register event because math worker is already running
	select {
	case e := <-events:
		if e.Action != protocol.Register {
			t.Fatalf("unexpected action: %s", e.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	t.Log("closing mathworker")
	mathKite.Close()

	// We must get Deregister event
	select {
	case e := <-events:
		if e.Action != protocol.Deregister {
			t.Fatalf("unexpected action: %s", e.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// Start a new mathworker kite
	t.Log("Setting up mathworker2")
	mathKite2 := kite.New("mathworker", "1.2.3")
	mathKite2.Config = conf.Copy()
	mathKite2.Start()

	reg2 := registration.New(mathKite2)
	go reg2.RegisterToProxyAndKontrol()
	<-reg2.ReadyNotify()

	// We must get Register event
	select {
	case e := <-events:
		if e.Action != protocol.Register {
			t.Fatalf("unexpected action: %s", e.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	err = watcher.Cancel()
	if err != nil {
		t.Fatal(err)
	}

	// We must not get any event after cancelling the watcher
	select {
	case e := <-events:
		t.Fatalf("unexpected event: %s", e)
	case <-time.After(time.Second):
	}
}

func Square(r *kite.Request) (interface{}, error) {
	a, err := r.Args.One().Float64()
	if err != nil {
		return nil, err
	}

	result := a * a

	fmt.Printf("Kite call, sending result '%f' back\n", result)

	return result, nil
}

func TestGetQueryKey(t *testing.T) {
	// This query is valid because there are no gaps between query fields.
	q := &protocol.KontrolQuery{
		Username:    "cenk",
		Environment: "production",
	}
	key, err := getQueryKey(q)
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
	key, err = getQueryKey(q)
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
	key, err = getQueryKey(q)
	if err == nil {
		t.Errorf("Error is expected")
	}
	if key != "" {
		t.Errorf("Key is not expected: %s", key)
	}
}
