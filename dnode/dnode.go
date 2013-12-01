// Package dnode implements a message processor for communication
// via dnode protocol. See the following URL for details:
// https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown
package dnode

import (
	"errors"
	"io/ioutil"
	"log"
	"reflect"
)

var l *log.Logger = log.New(ioutil.Discard, "", log.Lshortfile)

// Uncomment following to see log messages.
// var l *log.Logger = log.New(os.Stderr, "", log.Lshortfile)

type Dnode struct {
	// Registered methods are saved in this map.
	handlers map[string]Handler

	// Reference to sent callbacks are saved in this map.
	callbacks map[uint64]Handler

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
	// Wrap arguments before sending to remote.
	WrapArgs(args []interface{}, tr Transport) []interface{}

	// Unwrap arguments and call the handler function.
	Call(method string, args *Partial, tr Transport)
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
		callbacks: make(map[uint64]Handler),
		transport: transport,
	}
}

// Copy returns a pointer to a new Dnode with the same handlers as d but empty callbacks.
func (d *Dnode) Copy(transport Transport) *Dnode {
	return &Dnode{
		handlers:  d.handlers,
		callbacks: make(map[uint64]Handler),
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
func (d *Dnode) HandleFunc(method string, handler func(string, *Partial, Transport)) {
	d.Handle(method, HandlerFunc(handler))
}

type HandlerFunc func(method string, args *Partial, tr Transport)

func (f HandlerFunc) Call(method string, args *Partial, tr Transport) {
	f(method, args, tr)
}

func (f HandlerFunc) WrapArgs(args []interface{}, tr Transport) []interface{} {
	return args
}

// HandleSimple registers the handler function for given method.
// The difference from HandleFunc() that all dnode message arguments are passed
// directly to the handler instead of Message and Transport.
func (d *Dnode) HandleSimple(method string, handler interface{}) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		panic(errors.New("dnode: handler is not a func"))
	}

	d.Handle(method, SimpleFunc(v))
}

type SimpleFunc reflect.Value

func (f SimpleFunc) Call(method string, args *Partial, tr Transport) {
	// Call the handler with arguments.
	callArgs := []reflect.Value{reflect.ValueOf(args)}
	reflect.Value(f).Call(callArgs)
}

func (f SimpleFunc) WrapArgs(args []interface{}, tr Transport) []interface{} {
	return args
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

// RemoveCallback removes the callback with id from callbacks.
// Can be used to remove unused callbacks to free memory.
func (d *Dnode) RemoveCallback(id uint64) {
	delete(d.callbacks, id)
}
