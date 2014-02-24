package kontrol

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kontrolclient"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/simple"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
)

func TestKontrol(t *testing.T) {
	testutil.WriteKiteKey()

	kon := New(testkeys.Public, testkeys.Private)
	kon.DataDir = os.TempDir()
	defer os.RemoveAll(kon.DataDir)
	kon.Start()
	kon.ClearKites()

	mathKite := simple.New("mathworker", "0.0.1")
	mathKite.HandleFunc("square", Square)
	mathKite.Run()
	go http.ListenAndServe("127.0.0.1:3636", mathKite)

	exp2Kite := kite.New("exp2", "0.0.1")
	go http.ListenAndServe("127.0.0.1:3637", exp2Kite)

	// Wait for kites to register themselves on Kontrol.
	time.Sleep(500 * time.Millisecond)

	query := protocol.KontrolQuery{
		Username:    "testuser",
		Environment: "development",
		Name:        "mathworker",
	}

	konClient := kontrolclient.New(exp2Kite)
	kites, err := konClient.GetKites(query)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	if len(kites) == 0 {
		t.Errorf("No mathworker available")
		return
	}

	remoteMathWorker := kites[0]
	err = remoteMathWorker.Dial()
	if err != nil {
		t.Errorf("Cannot connect to remote mathworker")
		return
	}

	// Test Kontrol.GetToken
	fmt.Printf("oldToken: %#v\n", remoteMathWorker.Authentication.Key)
	newToken, err := konClient.GetToken(&remoteMathWorker.Kite)
	if err != nil {
		t.Errorf(err.Error())
	}
	fmt.Printf("newToken: %#v\n", newToken)

	// Run "square" method
	response, err := remoteMathWorker.Tell("square", 2)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	var result int
	err = response.Unmarshal(&result)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	// Result must be "4"
	if result != 4 {
		t.Errorf("Invalid result: %d", result)
		return
	}

	events := make(chan *kontrolclient.Event, 3)

	// Test WatchKites
	watcher, err := konClient.WatchKites(query, func(e *kontrolclient.Event, err error) {
		if err != nil {
			t.Fatalf(err.Error())
		}

		t.Logf("Event.Action: %s Event.Kite.ID: %s", e.Action, e.Kite.ID)
		events <- e
	})
	if err != nil {
		t.Errorf("Cannot watch: %s", err.Error())
		return
	}

	// First event must be register event because math worker is already running
	select {
	case e := <-events:
		if e.Action != protocol.Register {
			t.Errorf("unexpected action: %s", e.Action)
			return
		}
	case <-time.After(time.Second):
		t.Error("timeout")
		return
	}

	mathKite.Close()

	// We must get Deregister event
	select {
	case e := <-events:
		if e.Action != protocol.Deregister {
			t.Errorf("unexpected action: %s", e.Action)
			return
		}
	case <-time.After(time.Second):
		t.Error("timeout")
		return
	}

	// Start a new mathworker kite
	mathKite = mathWorker()
	mathKite.Start()

	// We must get Register event
	select {
	case e := <-events:
		if e.Action != protocol.Register {
			t.Errorf("unexpected action: %s", e.Action)
			return
		}
	case <-time.After(time.Second):
		t.Error("timeout")
		return
	}

	err = watcher.Cancel()
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	// We must not get any event after cancelling the watcher
	select {
	case e := <-events:
		t.Errorf("unexpected event: %s", e)
		return
	case <-time.After(time.Second):
	}
}

func Square(r *kite.Request) (interface{}, error) {
	a, err := r.Args[0].Float64()
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
