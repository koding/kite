package dnode

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestSimpleMethodCall(t *testing.T) {
	called := false

	receiver := New()
	receiver.HandleFunc("print", func(msg string) { fmt.Println(msg); called = true })
	go receiver.Run()
	defer receiver.Close()

	sender := New()
	go sender.Run()
	defer sender.Close()

	go sender.Call("print", "hello world")
	receiver.ReceiveChan <- <-sender.SendChan
	sleep()

	if !called {
		t.Error("Function is not called")
	}
}

func TestMethodCallWithCallback(t *testing.T) {
	var result float64 = 0
	success := func(code float64) { fmt.Println("success"); result = code }
	failure := func(code float64) { fmt.Println("failure"); result = -code }

	receiver := New()
	foo := func(success, failure Callback) { success(6) }
	receiver.HandleFunc("foo", foo)
	go receiver.Run()
	defer receiver.Close()

	sender := New()
	go sender.Run()
	defer sender.Close()

	go sender.Call("foo", success, failure)
	receiver.ReceiveChan <- <-sender.SendChan
	sender.ReceiveChan <- <-receiver.SendChan
	sleep()

	if result != 6 {
		t.Error("success callback is not called")
	}
}

func TestSend(t *testing.T) {
	d := New()

	// Send a single string method.
	go d.Call("echo", "hello", "world")
	expected := `{"method":"echo","arguments":["hello","world"],"callbacks":{},"links":[]}`
	err := assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a single integer method.
	go d.Call(5, "hello", "world")
	expected = `{"method":5,"arguments":["hello","world"],"callbacks":{},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestSendCallback(t *testing.T) {
	d := New()

	// echo function also sends the messages to this channel so
	// we can assert the call and passed arguments.
	echoChan := make(chan string)
	echo := func(msg string) {
		fmt.Println(msg)
		echoChan <- msg
	}

	mapChan := make(chan map[string]string)
	_ = func(m map[string]string) {
		fmt.Printf("map: %#v\n", m)
		mapChan <- m
	}

	// Test a single callback function.
	go d.Call("echo", echo)
	expected := `{"method":"echo","arguments":["[Function]"],"callbacks":{"0":["0"]},"links":[]}`
	err := assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a second method and see that callback number is increased by one.
	go d.Call("echo", echo)
	expected = `{"method":"echo","arguments":["[Function]"],"callbacks":{"1":["0"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a string and a callback as an argument.
	go d.Call("echo", "hello cenk", echo)
	expected = `{"method":"echo","arguments":["hello cenk","[Function]"],"callbacks":{"2":["1"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a string and a callback as an argument.
	go d.Call("echo", map[string]interface{}{"fn": echo, "msg": "hello cenk"})
	expected = `{"method":"echo","arguments":[{"fn":"[Function]","msg":"hello cenk"}],"callbacks":{"3":["0","fn"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Same above with a pointer to map.
	go d.Call("echo", &map[string]interface{}{"fn": echo, "msg": "hello cenk"})
	expected = `{"method":"echo","arguments":[{"fn":"[Function]","msg":"hello cenk"}],"callbacks":{"4":["0","fn"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// For testing sending a struct with methods
	a := Math{
		Name: "Pisagor",
		i:    6,
	}

	// Send the struct itself.
	// Pointer receivers will not be accessible.
	go d.Call("calculate", a, 2)
	expected = `{"method":"calculate","arguments":[{"Name":"Pisagor"},2],"callbacks":{"5":["0","add"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a pointer to struct.
	// Pointer receivers will be accessible.
	go d.Call("calculate", &a, 2)
	expected = `{"method":"calculate","arguments":[{"Name":"Pisagor"},2],"callbacks":{"6":["0","add"],"7":["0","subtract"]},"links":[]}`
	err = assertSentMessage(d.SendChan, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Process first callback method.
	var msg Message
	call := `{"method": 0, "arguments": ["hello cenk"]}`
	json.Unmarshal([]byte(call), &msg)
	go d.processMessage(msg)
	expected = "hello cenk"
	err = assertCallbackIsCalled(echoChan, expected)
	if err != nil {
		t.Error(err)
		return
	}
}

func assertSentMessage(ch chan Message, expected string) error {
	// Receive from SendChannel and assert the message.
	select {
	case msg := <-ch:
		b, _ := json.Marshal(msg)
		s := string(b)
		fmt.Println("Sent", s)

		if s != expected {
			return fmt.Errorf("\nInvalid message : %s\nExpected message: %s", s, expected)
		}
	case <-time.After(10 * time.Millisecond):
		return fmt.Errorf("Did not receive a message from SendChan")
	}

	// SendChannel must be empty.
	select {
	case msg := <-ch:
		return fmt.Errorf("SendChan is not empty: %#v", msg)
	default:
	}

	return nil
}

func assertCallbackIsCalled(ch chan string, expected string) error {
	select {
	// A call is made.
	case s := <-ch:
		fmt.Println("Called with:", s)

		if s != expected {
			return fmt.Errorf("Invalid argument: %s", s)
		}
	// Nothing happened.
	case <-time.After(10 * time.Millisecond):
		return fmt.Errorf("Callback function is not called")
	}

	// Callback must be called once.
	select {
	case <-ch:
		return fmt.Errorf("Callback is called more than once.")
	default:
	}

	return nil
}

type Math struct {
	Name string
	i    int
}

// Value receiver
func (m Math) Add(val int) int {
	return m.i + val
}

// Pointer receiver
func (m *Math) Subtract(val int) int {
	return m.i - val
}

func sleep() { time.Sleep(100 * time.Millisecond) }
