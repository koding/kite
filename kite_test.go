package kite

import (
	"net/http"
	"testing"
	"time"

	"github.com/koding/kite/dnode"
	_ "github.com/koding/kite/testutil"
)

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	mathKite := New("mathworker", "0.0.1")
	mathKite.HandleFunc("square", Square)
	mathKite.HandleFunc("squareCB", SquareCB)
	mathKite.Config.DisableAuthentication = true
	go http.ListenAndServe("127.0.0.1:3636", mathKite)

	exp2Kite := New("exp2", "0.0.1")
	go http.ListenAndServe("127.0.0.1:3637", exp2Kite)

	// Wait until they start serving
	time.Sleep(time.Second)

	fooChan := make(chan string)
	handleFoo := func(r *Request) (interface{}, error) {
		s := r.Args.One().MustString()
		t.Logf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	}

	exp2Kite.HandleFunc("foo", handleFoo)

	// exp2 connects to mathworker
	remote := exp2Kite.NewClientString("ws://127.0.0.1:3636")
	err := remote.Dial()
	if err != nil {
		t.Fatal(err)
	}

	result, err := remote.Tell("square", 2)
	if err != nil {
		t.Fatal(err)
	}

	number := result.MustFloat64()

	t.Logf("rpc result: %f\n", number)

	if number != 4 {
		t.Fatalf("Invalid result: %f", number)
	}

	select {
	case s := <-fooChan:
		if s != "bar" {
			t.Fatalf("Invalid message: %s", s)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not get the message")
	}

	resultChan := make(chan float64, 1)
	resultCallback := func(args *dnode.Partial) {
		n := args.One().MustFloat64()
		resultChan <- n
	}

	result, err = remote.Tell("squareCB", 3, Callback(resultCallback))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case n := <-resultChan:
		if n != 9.0 {
			t.Fatalf("Unexpected result: %f", n)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not get the message")
	}
}

// Returns the result. Also tests reverse call.
func Square(r *Request) (interface{}, error) {
	a := r.Args.One().MustFloat64()
	result := a * a

	r.LocalKite.Log.Info("Kite call, sending result '%f' back\n", result)

	// Reverse method call
	r.Client.Go("foo", "bar")

	return result, nil
}

// Calls the callback with the result. For testing requests with Callback.
func SquareCB(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	a := args[0].MustFloat64()
	cb := args[1].MustFunction()

	result := a * a

	r.LocalKite.Log.Info("Kite call, sending result '%f' back\n", result)

	// Send the result.
	err := cb(result)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
