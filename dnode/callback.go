package dnode

import (
	"errors"
	"strconv"
)

// Function is the type for sending and receiving functions in dnode messages.
type Function struct {
	Caller caller
}

type caller interface {
	Call(args ...interface{}) error
}

// Call the received function.
func (f Function) Call(args ...interface{}) error {
	if !f.IsValid() {
		return errors.New("invalid function")
	}
	return f.Caller.Call(args...)
}

// IsValid returns true if f represents a Function.
// It returns false if f is the zero Value.
func (f Function) IsValid() bool {
	return f.Caller != nil
}

func (f Function) MarshalJSON() ([]byte, error) {
	if _, ok := f.Caller.(callback); !ok {
		return []byte(`null`), nil
	}
	return []byte(`"[Function]"`), nil
}

func (*Function) UnmarshalJSON(data []byte) error {
	return nil
}

// Callback is the wrapper for function when sending.
func Callback(f func(*Partial)) Function {
	return Function{
		Caller: callback(f),
	}
}

type callback func(*Partial)

func (f callback) Call(args ...interface{}) error {
	// Callback is only for sending functions to the remote side
	panic("you cannot call your own callback method")
}

// functionReceived is a type implementing caller interface.
// It is used to set the Function when a callback function is received.
type functionReceived func(...interface{}) error

func (f functionReceived) Call(args ...interface{}) error {
	return f(args...)
}

// CallbackSpec is a structure encapsulating a Function and it's Path.
// It is the type of the values in callbacks map.
type CallbackSpec struct {
	// Path represents the callback's path in the arguments structure.
	Path     Path
	Function Function
}

// Path represents a callback function's path in the arguments structure.
// Contains mixture of string and integer values.
type Path []interface{}

// parseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func ParseCallbacks(msg *Message, sender func(id uint64, args []interface{}) error) error {
	// Parse callbacks field and create callback functions.
	for methodID, path := range msg.Callbacks {
		id, err := strconv.ParseUint(methodID, 10, 64)
		if err != nil {
			return err
		}

		f := func(args ...interface{}) error { return sender(id, args) }
		spec := CallbackSpec{path, Function{functionReceived(f)}}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}

	return nil
}
