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
	add := func(p *dnode.Partial) {
		args, _ := p.Array()
		a := args[0].(float64)
		b := args[1].(float64)
		result := args[2].(dnode.Function)
		result(a + b)
	}
	s.HandleSimple("add", add)

	// Listen HTTP
	http.Handle("/dnode", s)
	go http.ListenAndServe(":5000", nil)
	sleep()

	// Connect to server
	c, err := Dial("ws://127.0.0.1:5000/dnode", false)
	if err != nil {
		t.Error(err)
		return
	}
	defer c.Close()

	// Call a method
	c.Call("add", 1, 2, func(p *dnode.Partial) {
		var r float64
		p.Unmarshal(&r)
		fmt.Println("Add result:", r)
	})

	sleep()
}

func sleep() { time.Sleep(100 * time.Millisecond) }
