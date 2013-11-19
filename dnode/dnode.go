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
	// Registered methods are saved in this map.
	handlers map[string]Handler

	// Reference to sent callbacks are saved in this map.
	callbacks map[uint64]SimpleFunc

	// Next callback number.
	// Incremented atomically by registerCallback().
	seq uint64

	// For sending and receiving messages
	transport Transport
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

// Objects implementing the Handler interface can be
// registered to serve a particular method in the dnode processor.
type Handler interface {
	ProcessMessage(*Message, Transport)
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
		handlers:  make(map[string]Handler),
		callbacks: make(map[uint64]SimpleFunc),
		transport: transport,
	}
}

// Copy returns a pointer to a new Dnode with the same handlers as d but empty callbacks.
func (d *Dnode) Copy(transport Transport) *Dnode {
	return &Dnode{
		handlers:  d.handlers,
		callbacks: make(map[uint64]SimpleFunc),
		transport: transport,
	}
}

// Handle registers the handler for the given method.
// If a handler already exists for method, Handle panics.
func (d *Dnode) Handle(method string, handler Handler) {
	if method == "" {
		panic("dnode: invalid method " + method)
	}
	if handler == nil {
		panic("dnode: nil handler")
	}
	if _, ok := d.handlers[method]; ok {
		panic("dnode: handler already exists for method")
	}

	d.handlers[method] = handler
}

// HandleFunc registers the handler function for the given method.
func (d *Dnode) HandleFunc(method string, handler func(*Message, Transport)) {
	d.Handle(method, HandlerFunc(handler))
}

type HandlerFunc func(*Message, Transport)

func (f HandlerFunc) ProcessMessage(m *Message, tr Transport) {
	f(m, tr)
}

// HandleSimple registers the handler function for given method.
// The difference from HandleFunc() all dnode message arguments are passed
// directly to the handler.
func (d *Dnode) HandleSimple(method string, handler interface{}) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		panic(errors.New("dnode: handler is not a func"))
	}

	d.Handle(method, SimpleFunc(v))
}

type SimpleFunc reflect.Value

func (f SimpleFunc) ProcessMessage(m *Message, tr Transport) {
	// Call the handler with arguments.
	args := []reflect.Value{reflect.ValueOf(m.Arguments)}
	reflect.Value(f).Call(args)
}

// Run processes incoming messages. Blocking.
func (d *Dnode) Run() error {
	for {
		msg, err := d.transport.Receive()
		if err != nil {
			return err
		}

		go d.processMessage(msg)
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

// registerCallback is called when a function/method is found in arguments array.
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
	d.callbacks[next] = SimpleFunc(callback)
}

// processMessage processes a single message and call the previously
// added callbacks.
func (d *Dnode) processMessage(data []byte) error {
	l.Printf("processMessage: %s", string(data))

	var (
		err     error
		msg     Message
		handler Handler
	)

	defer func() {
		if err != nil {
			l.Printf("Cannot process message: %s", err)
		}
	}()

	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	if err = d.ParseCallbacks(&msg); err != nil {
		return err
	}

	// Get the handler function. Method may be string or integer.
	l.Printf("Received method: %s", msg.Method)
	switch method := msg.Method.(type) {
	case float64:
		handler = d.callbacks[uint64(method)]
	case string:
		handler = d.handlers[method]
	default:
		err = fmt.Errorf("Invalid method: %s", msg.Method)
		return err
	}

	// Method is not found.
	if handler == nil {
		err = fmt.Errorf("Unknown method: %v", msg.Method)
		return err
	}

	handler.ProcessMessage(&msg, d.transport)

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
