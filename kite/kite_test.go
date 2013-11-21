package kite

import (
	"fmt"
	"koding/newkite/protocol"
	"testing"
	"time"
)

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	mathKite := mathWorker()
	go mathKite.Run()

	exp2Kite := exp2()
	go exp2Kite.Run()

	fooChan := make(chan string)
	handleFoo := func(r *Request) (interface{}, error) {
		s, _ := r.Args.String()
		fmt.Printf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	}

	exp2Kite.HandleFunc("foo", handleFoo)

	time.Sleep(100 * time.Millisecond)

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

	response, err := remote.Call("square", 2)
	if err != nil {
		fmt.Println(err)
		return
	}

	var result int
	err = response.Unmarshal(&result)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("rpc result: %d\n", result)

	if result != 4 {
		t.Errorf("Invalid result: %d", result)
	}

	select {
	case s := <-fooChan:
		if s != "bar" {
			t.Errorf("Invalid message: %s", s)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Did not get the message")
	}
}

func exp2() *Kite {
	options := &protocol.Options{
		Kitename:    "exp2",
		Version:     "1",
		Port:        "3637",
		Region:      "localhost",
		Environment: "development",
	}

	return New(options)
}

func mathWorker() *Kite {
	options := &protocol.Options{
		Kitename:    "mathworker",
		Version:     "1",
		Port:        "3636",
		Region:      "localhost",
		Environment: "development",
	}

	k := New(options)
	k.HandleFunc("square", Square)
	return k
}

func Square(r *Request) (interface{}, error) {
	a, err := r.Args.Float64()
	if err != nil {
		return nil, err
	}

	result := a * a

	fmt.Printf("Kite call, sending result '%s' back\n", result)

	// Reverse method call
	r.RemoteKite.Go("foo", "bar")

	return result, nil
}
