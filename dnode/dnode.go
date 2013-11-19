// Package dnode implements a message processor for communication
// via dnode protocol. See the following URL for details:
// https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown
package dnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strconv"
	"sync/atomic"
)

var l *log.Logger = log.New(ioutil.Discard, "", log.Lshortfile)

// Uncomment following to see log messages.
// var l *log.Logger = log.New(os.Stderr, "", log.Lshortfile)

type Dnode struct {
	// Registered methods with HandleFunc() are saved in this map with string keys.
	// Callback methods sent by Call() are saved here with integer keys.
	// Contains kinds of reflect.Func.
	handlers map[string]reflect.Value

	// Next callback number.
	// Incremented atomically by registerCallback().
	seq uint64

	// For sending and receiving messages
	transport Transport

	// If the method is not found in handlers the message will be forwarded to this
	ExternalHandler MessageHandler
}

// Transport is an interface for sending and receiving data on network.
// Each Transport must be unique for each Client.
type Transport interface {
	// Address of the connected client
	RemoteAddr() string

	// Send single message
	Send(msg []byte) error

	// Receive single message
	Receive() ([]byte, error)

	// A place to save/read extra information about the client
	Properties() map[string]interface{}
}

// MessageHandler is the interface for delegating message processing to outside
// of the Dnode instance.
type MessageHandler interface {
	HandleDnodeMessage(*Message, *Dnode, Transport) error
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

		err = d.processMessage(msg)
		if err != nil {
			fmt.Printf("Could not process message: %s\n", err)
		}
	}
}

// Call sends the method and arguments to remote.
func (d *Dnode) Call(method string, arguments ...interface{}) (map[string]Path, error) {
	if method == "" {
		panic("Empty method name")
	}

	return d.call(method, arguments...)
}

func (d *Dnode) call(method interface{}, arguments ...interface{}) (map[string]Path, error) {
	l.Printf("Call method: %s arguments: %+v\n", method, arguments)

	var err error
	callbacks := make(map[string]Path)
	defer func() {
		if err != nil {
			d.removeCallbacks(callbacks)
		}
	}()

	d.collectCallbacks(arguments, make(Path, 0), callbacks)

	// Do not encode empty arguments as "null", make it "[]".
	if arguments == nil {
		arguments = make([]interface{}, 0)
	}

	rawArgs, err := json.Marshal(arguments)
	if err != nil {
		l.Printf("Cannot marshal arguments: %s: %#v", err, arguments)
		return nil, err
	}

	msg := Message{
		Method:    method,
		Arguments: &Partial{Raw: rawArgs},
		Callbacks: callbacks,
		Links:     []interface{}{},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		l.Printf("Cannot marshal message: %s: %#v", err, msg)
		return nil, err
	}

	err = d.transport.Send(data)
	if err != nil {
		l.Printf("Cannot send message over transport: %s", err)
		return nil, err
	}

	// We are returning callbacks here so the caller can Cull() after it gets the response.
	return callbacks, nil
}

// Used to remove callbacks after error occurs in call().
func (d *Dnode) removeCallbacks(callbacks map[string]Path) {
	for id, _ := range callbacks {
		delete(d.handlers, id)
	}
}

// RemoveCallback removes the callback with id from handlers.
// Can be used to remove unused callbacks to free memory.
func (d *Dnode) RemoveCallback(id uint64) {
	delete(d.handlers, strconv.FormatUint(id, 10))
}

// collectCallbacks walks over the rawObj and populates callbackMap
// with callbacks. This is a recursive function. The top level call must
// sends arguments as rawObj, an empty path and empty callbackMap parameter.
func (d *Dnode) collectCallbacks(rawObj interface{}, path Path, callbackMap map[string]Path) {
	switch obj := rawObj.(type) {
	// skip nil values
	case nil:
	case []interface{}:
		for i, item := range obj {
			d.collectCallbacks(item, append(path, strconv.Itoa(i)), callbackMap)
		}
	case map[string]interface{}:
		for key, item := range obj {
			d.collectCallbacks(item, append(path, key), callbackMap)
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

		switch v.Kind() {
		case reflect.Func:
			d.registerCallback(v, path, callbackMap)
		case reflect.Ptr:
			e := v.Elem()
			if e == reflect.ValueOf(nil) {
				return
			}

			v = reflect.ValueOf(e.Interface())
			d.collectFields(v, path, callbackMap)
		case reflect.Struct:
			d.collectFields(v, path, callbackMap)
		}
	}
}

// collectFields collects callbacks from the exported fields of a struct.
func (d *Dnode) collectFields(v reflect.Value, path Path, callbackMap map[string]Path) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.PkgPath == "" { // exported
			d.collectCallbacks(v.Field(i).Interface(), append(path, f.Name), callbackMap)
		}
	}
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
	l.Printf("processMessage: %s", string(data))

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	if err := d.ParseCallbacks(&msg); err != nil {
		return err
	}

	// Method may be string or integer.
	l.Printf("All handlers: %#v", d.handlers)
	method := fmt.Sprint(msg.Method)

	// Get the handler function.
	l.Printf("Received method: %s", method)
	handler := d.handlers[method]

	// Try to find the handler from ExternalHandler.
	if handler == reflect.ValueOf(nil) && d.ExternalHandler != nil {
		l.Printf("Looking in external handler")
		go d.ExternalHandler.HandleDnodeMessage(&msg, d, d.transport)
		return nil
	}

	// Method is not found.
	if handler == reflect.ValueOf(nil) {
		return fmt.Errorf("Unknown method: %v", msg.Method)
	}

	// Call the handler with arguments.
	args := []reflect.Value{reflect.ValueOf(msg.Arguments)}
	go handler.Call(args)

	return nil
}

// ParseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func (d *Dnode) ParseCallbacks(msg *Message) error {
	// Parse callbacks field and create callback functions.
	l.Printf("Received message callbacks: %#v", msg.Callbacks)

	for methodID, path := range msg.Callbacks {
		l.Printf("MehodID: %s", methodID)

		id, err := strconv.ParseUint(methodID, 10, 64)
		if err != nil {
			return err
		}

		// When the callback is called, we must send the method to the remote.
		f := Function(func(args ...interface{}) error {
			_, err := d.call(id, args...)
			return err
		})

		spec := CallbackSpec{path, f}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}

	return nil
}
