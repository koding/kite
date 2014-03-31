package dnode

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

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
		panic("cannot happen")
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

func (c *CallbackSpec) Apply(value reflect.Value) error {
	i := 0
	for {
		switch value.Kind() {
		case reflect.Slice:
			if i == len(c.Path) {
				return fmt.Errorf("Callback path too short: %v", c.Path)
			}

			// Path component may be a string or an integer.
			var index int
			var err error
			switch v := c.Path[i].(type) {
			case string:
				index, err = strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("Integer expected in callback path, got '%v'.", c.Path[i])
				}
			case float64:
				index = int(v)
			default:
				panic(fmt.Errorf("Unknown type: %#v, %T", c.Path[i], c.Path[i]))
			}

			value = value.Index(index)
			i++
		case reflect.Map:
			if i == len(c.Path) {
				return fmt.Errorf("Callback path too short: %v", c.Path)
			}
			if i == len(c.Path)-1 && value.Type().Elem().Kind() == reflect.Interface {
				value.SetMapIndex(reflect.ValueOf(c.Path[i]), reflect.ValueOf(c.Function))
				return nil
			}
			value = value.MapIndex(reflect.ValueOf(c.Path[i]))
			i++
		case reflect.Ptr:
			value = value.Elem()
		case reflect.Interface:
			if i == len(c.Path) {
				value.Set(reflect.ValueOf(c.Function))
				return nil
			}
			value = value.Elem()
		case reflect.Struct:
			if value.Type() == reflect.TypeOf(Function{}) {
				caller := value.FieldByName("Caller")
				caller.Set(reflect.ValueOf(c.Function.Caller))
				return nil
			}

			if innerPartial, ok := value.Addr().Interface().(*Partial); ok {
				spec := CallbackSpec{c.Path[i:], c.Function}
				innerPartial.CallbackSpecs = append(innerPartial.CallbackSpecs, spec)
				return nil
			}

			// Path component may be a string or an integer.
			name, ok := c.Path[i].(string)
			if !ok {
				return fmt.Errorf("Invalid path: %#v", c.Path[i])
			}

			value = value.FieldByName(strings.ToUpper(name[0:1]) + name[1:])
			i++
		case reflect.Func:
			value.Set(reflect.ValueOf(c.Function))
			return nil
		case reflect.Invalid:
			// callback path does not exist, skip
			return nil
		default:
			return fmt.Errorf("Unhandled value of kind '%v' in callback path: %s", value.Kind(), value.Interface())
		}
	}
	return nil
}
