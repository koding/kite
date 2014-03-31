package dnode

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSimpleMethodCall(t *testing.T) {
	l := sync.Mutex{}
	called := false

	tr1 := newMockTransport()
	receiver := New(tr1)
	printFunc := func(args *Partial) {
		l.Lock()
		called = true
		l.Unlock()
		fmt.Println(string(args.One().Raw))
	}
	receiver.HandleFunc("print", printFunc)
	go receiver.Run()
	defer tr1.Close()

	tr2 := newMockTransport()
	sender := New(tr2)
	go sender.Run()
	defer tr2.Close()

	go sender.Call("print", "hello world")
	tr1.toReceive <- <-tr2.sent
	sleep()

	l.Lock()
	if !called {
		t.Error("Function is not called")
	}
	l.Unlock()
}

func TestMethodCallWithCallback(t *testing.T) {
	l := sync.Mutex{}
	var result float64
	successFunc := func(args *Partial) {
		fmt.Println("success")
		l.Lock()
		result, _ = args.One().Float64()
		l.Unlock()
	}
	failureFunc := func(args *Partial) {
		fmt.Println("failure")
		l.Lock()
		result, _ = args.One().Float64()
		result = -result
		l.Unlock()
	}

	tr1 := newMockTransport()
	receiver := New(tr1)
	fooFunc := func(args *Partial) {
		var callbacks []Function
		args.MustUnmarshal(&callbacks)
		callbacks[0](6)
	}
	receiver.HandleFunc("foo", fooFunc)
	go receiver.Run()
	defer tr1.Close()

	tr2 := newMockTransport()
	sender := New(tr2)
	go sender.Run()
	defer tr2.Close()

	go sender.Call("foo", Callback(successFunc), Callback(failureFunc))
	tr1.toReceive <- <-tr2.sent
	tr2.toReceive <- <-tr1.sent
	sleep()

	l.Lock()
	if result != 6 {
		t.Error("success callback is not called")
	}
	l.Unlock()
}

func TestCallMessage(t *testing.T) {
	tr := newMockTransport()
	d := New(tr)

	// Send a single string method.
	go d.Call("echo", "hello", "world")
	expected := `{"method":"echo","arguments":["hello","world"],"callbacks":{},"links":[]}`
	err := assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a single integer method.
	go d.send(5, []interface{}{"hello", "world"})
	expected = `{"method":5,"arguments":["hello","world"],"callbacks":{},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestSendCallback(t *testing.T) {
	tr := newMockTransport()
	d := New(tr)

	// echo function also sends the messages to this channel so
	// we can assert the call and passed arguments.
	echoChan := make(chan string)
	echoF := func(arguments *Partial) {
		msg := arguments.One().MustString()
		fmt.Println(msg)
		echoChan <- msg
	}
	echo := Callback(echoF)

	mapChan := make(chan map[string]string)
	_ = func(m map[string]string) {
		fmt.Printf("map: %#v\n", m)
		mapChan <- m
	}

	// Test a single callback function.
	go d.Call("echo", echo)
	expected := `{"method":"echo","arguments":["[Function]"],"callbacks":{"0":["0"]},"links":[]}`
	err := assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a second method and see that callback number is increased by one.
	go d.Call("echo", echo)
	expected = `{"method":"echo","arguments":["[Function]"],"callbacks":{"1":["0"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a string and a callback as an argument.
	go d.Call("echo", "hello cenk", echo)
	expected = `{"method":"echo","arguments":["hello cenk","[Function]"],"callbacks":{"2":["1"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a string and a callback as an argument.
	go d.Call("echo", map[string]interface{}{"fn": echo, "msg": "hello cenk"})
	expected = `{"method":"echo","arguments":[{"fn":"[Function]","msg":"hello cenk"}],"callbacks":{"3":["0","fn"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Same above with a pointer to map.
	go d.Call("echo", &map[string]interface{}{"fn": echo, "msg": "hello cenk"})
	expected = `{"method":"echo","arguments":[{"fn":"[Function]","msg":"hello cenk"}],"callbacks":{"4":["0","fn"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// For testing sending a struct with methods
	f := func(args *Partial) {}
	cb := Callback(f)
	a := Math{
		Name:      "Pisagor",
		i:         6,
		Callbacks: []interface{}{cb, 1, 2, 3},
		details:   []interface{}{cb, "x", "y", "z"},
	}

	// Send the struct itself.
	// Pointer receivers will not be accessible.
	go d.Call("calculate", a, 2)
	expected = `{"method":"calculate","arguments":[{"Name":"Pisagor","Callbacks":["[Function]",1,2,3]},2],"callbacks":{"5":["0","Callbacks","0"],"6":["0","add"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Send a pointer to struct.
	// Pointer receivers will be accessible.
	go d.Call("calculate", &a, 2)
	expected = `{"method":"calculate","arguments":[{"Name":"Pisagor","Callbacks":["[Function]",1,2,3]},2],"callbacks":{"7":["0","Callbacks","0"],"8":["0","add"],"9":["0","subtract"]},"links":[]}`
	err = assertSentMessage(tr.sent, expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Process first callback method.
	call := `{"method": 0, "arguments": ["hello cenk"]}`
	go d.processMessage([]byte(call))
	expected = "hello cenk"
	err = assertCallbackIsCalled(echoChan, expected)
	if err != nil {
		t.Error(err)
		return
	}
}

type mockTransport struct {
	sent      chan []byte
	toReceive chan []byte
	closeChan chan bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		sent:      make(chan []byte, 10),
		toReceive: make(chan []byte, 10),
		closeChan: make(chan bool, 1),
	}
}

func (t *mockTransport) Send(msg []byte) error {
	t.sent <- msg
	return nil
}

func (t *mockTransport) Receive() ([]byte, error) {
	select {
	case msg := <-t.toReceive:
		return msg, nil
	case <-t.closeChan:
		return nil, errors.New("closed")
	}
}

func (t *mockTransport) Close() {
	t.closeChan <- true
}

func (t *mockTransport) RemoteAddr() string {
	return "127.0.0.1:1234"
}

func (t *mockTransport) Properties() map[string]interface{} {
	return nil
}

func (t *mockTransport) Client() interface{} {
	return nil
}

func assertSentMessage(ch chan []byte, expected string) error {
	// Receive from SendChannel and assert the message.
	select {
	case msg := <-ch:
		s := string(msg)
		fmt.Println("Sent", s)

		if s != expected {
			return fmt.Errorf("\nInvalid message : %s\nExpected message: %s", s, expected)
		}
	case <-time.After(1000 * time.Millisecond):
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
	case <-time.After(1000 * time.Millisecond):
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
	// This will be exported
	Name string

	// This will be unexported
	i int

	// To test collecting callbacks in structs
	Callbacks []interface{}

	// Must not exported
	details []interface{}
}

// Value receiver
func (m Math) Add(val int) int {
	return m.i + val
}

// Pointer receiver
func (m *Math) Subtract(val int) int {
	return m.i - val
}

// This will not be exported
func (m *Math) asdf() int {
	return m.i
}

func sleep() { time.Sleep(100 * time.Millisecond) }
