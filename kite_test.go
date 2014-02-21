package kite

import (
	"fmt"
	"github.com/koding/kite/testutil"
	"testing"
	"time"
)

// This example shows a simple method call between two kites.
func Example() {
	// Start adder kite
	opts := &Options{
		Kitename:    "adder",
		Version:     "0.0.1",
		Environment: "development",
		Region:      "localhost",
		PublicIP:    "127.0.0.1",
		Port:        "10001",
		DisableAuthentication: true, // open to anyone
	}
	adder := New(opts)
	adder.HandleFunc("add", add)
	adder.Start()
	defer adder.Close()

	// Start foo kite
	opts = &Options{
		Kitename:    "foo",
		Version:     "0.0.1",
		Environment: "development",
		Region:      "localhost",
		PublicIP:    "127.0.0.1",
		Port:        "10002",
	}
	foo := New(opts)
	foo.Start()
	defer foo.Close()

	// foo kite calls the "add" method of adder kite
	remote := foo.NewRemoteKite(adder.Kite, Authentication{})
	err := remote.Dial()
	if err != nil {
		panic(err)
	}
	result, err := remote.Tell("add", 2, 3)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Result is %d\n", int(result.MustFloat64()))
	// Output: Result is 5
}

func add(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	return args[0].MustFloat64() + args[1].MustFloat64(), nil
}

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	testutil.WriteKiteKey()

	mathKite := mathWorker()
	mathKite.Start()
	defer mathKite.Close()

	exp2Kite := exp2()
	exp2Kite.Start()
	defer exp2Kite.Close()

	fooChan := make(chan string)
	handleFoo := func(r *Request) (interface{}, error) {
		s := r.Args.One().MustString()
		fmt.Printf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	}

	exp2Kite.HandleFunc("foo", handleFoo)

	// Use the kodingKey auth type since they are on same host.
	auth := Authentication{
		Type: "kiteKey",
		Key:  exp2Kite.kiteKey.Raw,
	}
	remote := exp2Kite.NewRemoteKite(mathKite.Kite, auth)

	err := remote.Dial()
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	result, err := remote.Tell("square", 2)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	number := result.MustFloat64()

	fmt.Printf("rpc result: %f\n", number)

	if number != 4 {
		t.Errorf("Invalid result: %f", number)
	}

	select {
	case s := <-fooChan:
		if s != "bar" {
			t.Errorf("Invalid message: %s", s)
			return
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Did not get the message")
		return
	}

	resultChan := make(chan float64, 1)
	resultCallback := func(r *Request) {
		fmt.Printf("Request: %#v\n", r)
		n := r.Args.One().MustFloat64()
		resultChan <- n
	}

	result, err = remote.Tell("square2", 3, Callback(resultCallback))
	if err != nil {
		t.Errorf(err.Error())
	}

	select {
	case n := <-resultChan:
		if n != 9.0 {
			t.Errorf("Unexpected result: %f", n)
			return
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Did not get the message")
		return
	}
}

func exp2() *Kite {
	options := &Options{
		Kitename:    "exp2",
		Version:     "0.0.1",
		Port:        "3637",
		Region:      "localhost",
		Environment: "development",
	}

	k := New(options)
	k.KontrolEnabled = false
	return k
}

func mathWorker() *Kite {
	options := &Options{
		Kitename:    "mathworker",
		Version:     "0.0.1",
		Port:        "3636",
		Region:      "localhost",
		Environment: "development",
	}

	k := New(options)
	k.KontrolEnabled = false
	k.HandleFunc("square", Square)
	k.HandleFunc("square2", Square2)
	return k
}

// Returns the result. Also tests reverse call.
func Square(r *Request) (interface{}, error) {
	a := r.Args[0].MustFloat64()
	result := a * a

	fmt.Printf("Kite call, sending result '%f' back\n", result)

	// Reverse method call
	r.RemoteKite.Go("foo", "bar")

	return result, nil
}

// Calls the callback with the result. For testing requests from Callback.
func Square2(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	a := args[0].MustFloat64()
	cb := args[1].MustFunction()

	result := a * a

	fmt.Printf("Kite call, sending result '%f' back\n", result)

	// Send the result.
	err := cb(result)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
