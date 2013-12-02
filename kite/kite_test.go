package kite

import (
	"fmt"
	"testing"
	"time"
)

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	mathKite := mathWorker()
	mathKite.Start()

	exp2Kite := exp2()
	exp2Kite.Start()

	fooChan := make(chan string)
	handleFoo := func(r *Request) (interface{}, error) {
		s := r.Args.MustString()
		fmt.Printf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	}

	exp2Kite.HandleFunc("foo", handleFoo)

	// Use the kodingKey auth type since they are on same host.
	auth := callAuthentication{
		Type: "kodingKey",
		Key:  exp2Kite.KodingKey,
	}
	remote := exp2Kite.NewRemoteKite(mathKite.Kite, auth)

	err := remote.Dial()
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	result, err := remote.Call("square", 2)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	number := result.MustFloat64()

	fmt.Printf("rpc result: %f\n", number)

	if number != 4 {
		t.Errorf("Invalid result: %d", number)
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
		args := r.Args.MustSliceOfLength(1)
		n := args[0].MustFloat64()
		resultChan <- n
	}

	args := []interface{}{3, Callback(resultCallback)}
	result, err = remote.Call("square2", args)
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
		Version:     "1",
		Port:        "3637",
		Region:      "localhost",
		Environment: "development",
	}

	k := New(options)
	return k
}

func mathWorker() *Kite {
	options := &Options{
		Kitename:    "mathworker",
		Version:     "1",
		Port:        "3636",
		Region:      "localhost",
		Environment: "development",
	}

	k := New(options)
	k.HandleFunc("square", Square)
	k.HandleFunc("square2", Square2)
	return k
}

// Returns the result. Also tests reverse call.
func Square(r *Request) (interface{}, error) {
	a := r.Args.MustFloat64()
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
