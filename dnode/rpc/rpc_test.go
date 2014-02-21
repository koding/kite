package rpc

import (
	"fmt"
	"github.com/koding/kite/dnode"
	"net/http"
	"testing"
	"time"
)

func TestClientServer(t *testing.T) {
	// Create new dnode server
	s := NewServer()
	add := func(p *dnode.Partial) {
		args, _ := p.Slice()
		a, _ := args[0].Float64()
		b, _ := args[1].Float64()
		result, _ := args[2].Function()
		result(a + b)
	}
	s.HandleFunc("add", add)

	// Listen HTTP
	http.Handle("/dnode", s)
	go http.ListenAndServe(":5050", nil)
	sleep()

	// Connect to server
	c, err := Dial("ws://127.0.0.1:5050/dnode", false)
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
