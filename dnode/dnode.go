// https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown
package dnode

import (
	"encoding/json"
	"errors"
	"fmt"
	_ "io/ioutil"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

const functionPlaceholder = "[Function]"

// var l *log.Logger = log.New(ioutil.Discard, "", log.Lshortfile)

var l *log.Logger = log.New(os.Stderr, "", log.Lshortfile)

type Dnode struct {
	// Registered mehtods with HandleFunc() are saved in this map with string keys.
	// Callback methods sent by Call() are saved here with integer keys.
	// Contains kinds of reflect.Func.
	handlers map[string]reflect.Value

	// Next callback number.
	// Incremented atomically by registerCallback().
	seq uint64

	// For sending and receiving messages
	transport Transport
}

type Transport interface {
	Send(msg []byte) error
	Receive() ([]byte, error)
}

// Message is the JSON object to call a method at the other side.
type Message struct {
	// Method can be an integer or string.
	Method interface{} `json:"method"`

	// Array of arguments
	Arguments *Partial `json:"arguments"`

	// Integer map of callback paths in arguments
	Callbacks map[string]Path `json:"callbacks"`

	// Links are not used for now.
	Links []interface{} `json:"links"`
}

// New returns a pointer to a new Dnode.
func New(transport Transport) *Dnode {
	return &Dnode{
		handlers:  make(map[string]reflect.Value),
		transport: transport,
	}
}

// HandleFunc registers a function.
func (d *Dnode) HandleFunc(method string, handler interface{}) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		panic(errors.New("handler is not a func"))
	}

	d.handlers[method] = v
}

// Run processes incoming messages. Blocking.
func (d *Dnode) Run() error {
	for {
		msg, err := d.transport.Receive()
		if err != nil {
			return err
		}

		d.processMessage(msg)
	}
}

// Send serializes the method and arguments, then sends to the SendChan.
// The user is responsible for reading from the channel and sending
// messages to the remote side.
func (d *Dnode) Call(method interface{}, arguments ...interface{}) error {
	l.Printf("Call method: %s arguments %+v\n", method, arguments)

	callbacks := make(map[string]Path)
	d.collectCallbacks(arguments, make(Path, 0), callbacks)

	rawArgs, err := json.Marshal(arguments)
	if err != nil {
		return err
	}

	msg := Message{
		Method:    method,
		Arguments: &Partial{Raw: rawArgs},
		Callbacks: callbacks,
		Links:     []interface{}{},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return d.transport.Send(data)
}

// collectCallbacks walks over the rawObj and populates callbackMap
// with callbacks. This is a recursive function. The top level call must
// sends arguments as rawObj, an empty path and empty callbackMap parameter.
func (d *Dnode) collectCallbacks(rawObj interface{}, path Path, callbackMap map[string]Path) bool {
	switch obj := rawObj.(type) {
	// skip nil values
	case nil:
	case []interface{}:
		for i, item := range obj {
			if d.collectCallbacks(item, append(path, strconv.Itoa(i)), callbackMap) {
				obj[i] = functionPlaceholder
			}
		}
	case map[string]interface{}:
		for key, item := range obj {
			if d.collectCallbacks(item, append(path, key), callbackMap) {
				obj[key] = functionPlaceholder
			}
		}
	// Dereference and continue.
	case *[]interface{}:
		if obj != nil {
			d.collectCallbacks(*obj, path, callbackMap)
		}
	// Dereference and continue.
	case *map[string]interface{}:
		if obj != nil {
			d.collectCallbacks(*obj, path, callbackMap)
		}
	default:
		v := reflect.ValueOf(obj)

		if v.Kind() == reflect.Func {
			d.registerCallback(v, path, callbackMap)
			return true
		}

		// If type has methods, register them.
		for i := 0; i < v.NumMethod(); i++ {
			m := v.Type().Method(i)
			if m.PkgPath == "" { // exported
				name := v.Type().Method(i).Name
				name = strings.ToLower(name[0:1]) + name[1:]
				d.registerCallback(v.Method(i), append(path, name), callbackMap)
			}
		}
	}

	return false
}

// registerCallback is called when a function/mehtod is found in arguments array.
func (d *Dnode) registerCallback(callback reflect.Value, path Path, callbackMap map[string]Path) {
	// Make a copy of path because it is reused in caller.
	pathCopy := make(Path, len(path))
	copy(pathCopy, path)

	// Subtract one to start counting from zero.
	// This is not absolutely necessary, just cosmetics.
	next := atomic.AddUint64(&d.seq, 1) - 1

	seq := strconv.FormatUint(next, 10)

	// Add to callback map to be sent to remote.
	callbackMap[seq] = pathCopy

	// Save in client callbacks so we can call it when we receive a call.
	d.handlers[seq] = callback
}

// processMessage processes a single message and call the previously
// added callbacks.
func (d *Dnode) processMessage(data []byte) error {
	var msg Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return err
	}

	l.Printf("processMessage: %#v", msg)

	// Parse callbacks field and create callback functions.
	l.Printf("Received message callbacks: %#v", msg.Callbacks)
	for methodID, path := range msg.Callbacks {
		l.Printf("MehodID", methodID)
		// When the callback is called, we must send the method to the remote.
		methodID2 := methodID // Closure issue, bind variable again.
		f := Callback(func(args ...interface{}) {
			d.Call(methodID2, args...)
		})
		spec := CallbackSpec{path, f}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}
	l.Printf("All callbackspecs: %#v", msg.Arguments.CallbackSpecs)

	// Method may be string or integer
	l.Printf("All handlers: %#v", d.handlers)
	method := fmt.Sprint(msg.Method)
	l.Printf("Received method: %s", method)
	handler := d.handlers[method]
	if handler.Kind() != reflect.Func {
		return fmt.Errorf("Unknown method: %v", msg.Method)
	}

	// Get arguments as array.
	args, err := msg.Arguments.Array()
	if err != nil {
		return err
	}
	l.Printf("Array of args: %#v", args)

	// Get the reflect.Value of arguments.
	callArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		callArgs[i] = reflect.ValueOf(arg)
	}

	l.Printf("Calling %#v with args: %+v\n", handler.String(), args)
	go handler.Call(callArgs)

	return nil
}
