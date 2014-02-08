package dnode

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

// Call sends the method and arguments to remote.
func (d *Dnode) Call(method string, arguments ...interface{}) (map[string]Path, error) {
	if method == "" {
		return nil, errors.New("Can't make a call with empty method name")
	}

	if arguments == nil {
		arguments = make([]interface{}, 0)
	}
	if d.WrapMethodArgs != nil {
		arguments = d.WrapMethodArgs(arguments, d.transport)
	}

	return d.send(method, arguments)
}

func (d *Dnode) send(method interface{}, arguments []interface{}) (map[string]Path, error) {
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
		return nil, err
	}

	err = d.transport.Send(data)
	if err != nil {
		return nil, err
	}

	// We are returning callbacks here so the caller can Cull() after it gets the response.
	return callbacks, nil
}

// Used to remove callbacks after error occurs in send().
func (d *Dnode) removeCallbacks(callbacks map[string]Path) {
	for id, _ := range callbacks {
		delete(d.handlers, id)
	}
}

// collectCallbacks walks over the rawObj and populates callbackMap
// with callbacks. This is a recursive function. The top level send must
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

			v2 := reflect.ValueOf(e.Interface())
			d.collectFields(v2, path, callbackMap)
			d.collectMethods(v, path, callbackMap)
		case reflect.Struct:
			d.collectFields(v, path, callbackMap)
			d.collectMethods(v, path, callbackMap)
		}
	}
}

// collectFields collects callbacks from the exported fields of a struct.
func (d *Dnode) collectFields(v reflect.Value, path Path, callbackMap map[string]Path) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)

		if f.PkgPath != "" { // unexported
			continue
		}

		// Do not collect callbacks for "-" tagged fields.
		tag := f.Tag.Get("dnode")
		if tag == "-" {
			continue
		}

		tag = f.Tag.Get("json")
		if tag == "-" {
			continue
		}

		var name string
		if tag != "" {
			name = tag
		} else {
			name = f.Name
		}

		if f.Anonymous {
			d.collectCallbacks(v.Field(i).Interface(), path, callbackMap)
		} else {
			d.collectCallbacks(v.Field(i).Interface(), append(path, name), callbackMap)
		}
	}
}

func (d *Dnode) collectMethods(v reflect.Value, path Path, callbackMap map[string]Path) {
	for i := 0; i < v.NumMethod(); i++ {
		if v.Type().Method(i).PkgPath == "" { // exported
			name := v.Type().Method(i).Name
			name = strings.ToLower(name[0:1]) + name[1:]
			d.registerCallback(v.Method(i), append(path, name), callbackMap)
		}
	}
}

// registerCallback is called when a function/method is found in arguments array.
func (d *Dnode) registerCallback(val reflect.Value, path Path, callbackMap map[string]Path) {
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
	d.callbacks[next] = val
}
