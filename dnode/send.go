package dnode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"sync/atomic"
)

// Call sends the method and arguments to remote.
func (d *Dnode) Call(method string, arguments ...interface{}) (map[string]Path, error) {
	if method == "" {
		panic("Empty method name")
	}

	return d.call(method, arguments...)
}

func (d *Dnode) call(method interface{}, arguments ...interface{}) (map[string]Path, error) {
	l.Printf("Call method: %s arguments: %+v\n", fmt.Sprint(method), arguments)

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

		name := f.Tag.Get("json")
		if name == "" {
			name = f.Name
		}

		if f.PkgPath == "" { // exported
			d.collectCallbacks(v.Field(i).Interface(), append(path, name), callbackMap)
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
	if fn, ok := val.Interface().(Handler); ok {
		d.callbacks[next] = fn
	} else {
		d.callbacks[next] = SimpleFunc(val)
	}
}
