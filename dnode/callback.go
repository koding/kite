package dnode

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const functionPlaceholder = `"[Function]"`

// Callback is a function to send in arguments.
// Wrap your function to make it marshalable before sending.
type Callback func(*Partial)

func (c Callback) MarshalJSON() ([]byte, error) {
	return []byte(functionPlaceholder), nil
}

// Funcion is a callable function with arbitrary args and error return value.
// It is used to wrap the callback function on receiving side.
type Function func(...interface{}) error

// UnmarshalJSON marshals the callback as "nil".
// Value of the callback is not important in dnode protocol.
// Skips unmarshal errors when unmarshalling a callback placeholder to Callback.
func (f *Function) UnmarshalJSON(data []byte) error {
	return nil
}

// Path represents a callback function's path in the arguments structure.
// Contains mixture of string and integer values.
type Path []interface{}

// CallbackSpec is a structure encapsulating a Function and it's Path.
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
			case int:
				index = v
			default:
				panic(fmt.Errorf("Unknown type: %#v", c.Path[i]))
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
