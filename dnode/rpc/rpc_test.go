package rpc

import (
	"fmt"
	"koding/newkite/dnode"
	"net/http"
	"testing"
	"time"
)

func TestClientServer(t *testing.T) {
	// Create new dnode server
	s := NewServer()
	add := func(a, b float64, result dnode.Callback) {
		result(a + b)
	}
	s.HandleFunc("add", add)

	// Listen HTTP
	http.Handle("/dnode", s)
	go http.ListenAndServe(":5000", nil)
	sleep()

	// Connect to server
	c, err := Dial("ws://127.0.0.1:5000/dnode")
	if err != nil {
		t.Error(err)
		return
	}
	defer c.Close()

	// Call a method
	c.Call("add", 1, 2, func(r float64) {
		fmt.Println("Add result:", r)
	})

	sleep()
}

func sleep() { time.Sleep(100 * time.Millisecond) }
