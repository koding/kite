package dnode

// Function is the type for sending and receiving functions in dnode messages.
type Function struct {
	Caller caller
}

// Call the received function.
func (f Function) Call(args ...interface{}) error {
	return f.Caller.Call(args...)
}

func (f Function) MarshalJSON() ([]byte, error) {
	if _, ok := f.Caller.(callback); !ok {
		return []byte(`"null"`), nil
	}
	return []byte(`"[Function]"`), nil
}

func (*Function) UnmarshalJSON(data []byte) error {
	return nil
}

type caller interface {
	Call(args ...interface{}) error
}

// Callback is the wrapper for function when sending.
func Callback(f func(*Partial)) Function {
	return Function{callback(f)}
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

// Path represents a callback function's path in the arguments structure.
// Contains mixture of string and integer values.
type Path []interface{}

// CallbackSpec is a structure encapsulating a Function and it's Path.
// It is the type of the values in callbacks map.
type CallbackSpec struct {
	// Path represents the callback's path in the arguments structure.
	Path     Path
	Function Function
}
