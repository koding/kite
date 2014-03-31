package kontrol

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/simple"
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
}

func TestKontrol(t *testing.T) {
	kon := New(conf.Copy(), testkeys.Public, testkeys.Private)
	kon.DataDir, _ = ioutil.TempDir("", "")
	defer os.RemoveAll(kon.DataDir)
	kon.Start()

	prx := proxy.New(conf.Copy(), testkeys.Public, testkeys.Private)
	prx.Start()

	time.Sleep(1e9)

	mathKite := simple.New("mathworker", "0.0.1")
	mathKite.Config = conf.Copy()
	mathKite.HandleFunc("square", Square)
	mathKite.Start()

	<-mathKite.Registration.ReadyNotify()

	exp2Kite := kite.New("exp2", "0.0.1")
	exp2Kite.Config = conf.Copy()

	query := protocol.KontrolQuery{
		Username:    exp2Kite.Kite().Username,
		Environment: exp2Kite.Kite().Environment,
		Name:        "mathworker",
	}

	konClient := kontrolclient.New(exp2Kite)
	connected, err := konClient.DialForever()
	if err != nil {
		t.Fatal(err)
	}

	<-connected

	kites, err := konClient.GetKites(query)
	if err != nil {
		t.Fatal(err)
	}

	if len(kites) == 0 {
		t.Fatal("No mathworker available")
	}

	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Fatal("Cannot connect to remote mathworker")
	}

	// Test Kontrol.GetToken
	t.Logf("oldToken: %s", remoteMathWorker.Authentication.Key)
	newToken, err := konClient.GetToken(&remoteMathWorker.Kite)
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

	events := make(chan *kontrolclient.Event, 3)

	// Test WatchKites
	watcher, err := konClient.WatchKites(query, func(e *kontrolclient.Event, err *kite.Error) {
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
	mathKite2 := simple.New("mathworker", "0.0.1")
	mathKite2.Config = conf.Copy()
	mathKite2.Start()

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
