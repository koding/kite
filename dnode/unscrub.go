package dnode

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func (s *Scrubber) Unscrub(arguments interface{}, callbacks map[string]Path, f func(uint64) functionReceived) error {
	v := reflect.ValueOf(arguments)
	if v.Kind() != reflect.Ptr {
		panic("arguments must be a pointer")
	}

	// Parse callbacks field and create callback functions.
	for sid, path := range callbacks {
		id, err := strconv.ParseUint(sid, 10, 64)
		if err != nil {
			return err
		}

		if err = setCallback(v, path, f(id)); err != nil {
			return err
		}
	}

	return nil
}

func setCallback(value reflect.Value, path Path, cb functionReceived) error {
	i := 0
	for {
		switch value.Kind() {
		case reflect.Slice:
			if i == len(path) {
				return fmt.Errorf("callback path too short: %v", path)
			}

			// Path component may be a string or an integer.
			var index int
			var err error
			switch v := path[i].(type) {
			case string:
				index, err = strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("integer expected in callback path, got '%v'.", path[i])
				}
			case float64:
				index = int(v)
			default:
				panic(fmt.Errorf("unknown type: %#v", path[i]))
			}

			value = value.Index(index)
			i++
		case reflect.Map:
			if i == len(path) {
				return fmt.Errorf("callback path too short: %v", path)
			}
			if i == len(path)-1 && value.Type().Elem().Kind() == reflect.Interface {
				value.SetMapIndex(reflect.ValueOf(path[i]), reflect.ValueOf(cb))
				return nil
			}
			value = value.MapIndex(reflect.ValueOf(path[i]))
			i++
		case reflect.Ptr:
			value = value.Elem()
		case reflect.Interface:
			if i == len(path) {
				value.Set(reflect.ValueOf(cb))
				return nil
			}
			value = value.Elem()
		case reflect.Struct:
			if value.Type() == reflect.TypeOf(Function{}) {
				caller := value.FieldByName("Caller")
				caller.Set(reflect.ValueOf(cb))
				return nil
			}

			if innerPartial, ok := value.Addr().Interface().(*Partial); ok {
				spec := CallbackSpec{path[i:], Function{cb}}
				innerPartial.CallbackSpecs = append(innerPartial.CallbackSpecs, spec)
				return nil
			}

			// Path component may be a string or an integer.
			name, ok := path[i].(string)
			if !ok {
				return fmt.Errorf("Invalid path: %#v", path[i])
			}

			value = value.FieldByName(strings.ToUpper(name[0:1]) + name[1:])
			i++
		case reflect.Func:
			// plain func is not supported, use Function type
			return nil
		case reflect.Invalid:
			// callback path does not exist, skip
			return nil
		default:
			return fmt.Errorf("Unhandled value of kind '%v' in callback path: %s", value.Kind(), value.Interface())
		}
	}
}
