package dnode

import (
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

// Scrub creates an object that represents "callbacks" field in dnode message.
// The obj argument which will be scrubbed must be of array, slice, struct, or
// map type. If structure is passed, the returned callbacks map will contain its
// exported methods of func(*Partial) signature. Other functions must be
// wrapped by Callback function.
func (s *Scrubber) Scrub(obj interface{}) (callbacks map[string]Path) {
	callbacks = make(map[string]Path)
	rv := reflect.ValueOf(obj)

	k := rv.Kind()
	if k != reflect.Array && k != reflect.Slice && k != reflect.Struct && k != reflect.Map {
		return nil
	}

	s.collect(rv, make(Path, 0), callbacks)
	return callbacks
}

var dnodeFunctionType = reflect.TypeOf(new(Function)).Elem()

func (s *Scrubber) collect(rv reflect.Value, path Path, callbacks map[string]Path) {
	switch rv.Kind() {
	case reflect.Interface:
		if !rv.IsNil() {
			s.collect(rv.Elem(), path, callbacks)
		}
	case reflect.Ptr:
		if rv.IsNil() {
			return
		}
		// collect from structs that define pointer reciver methods.
		if elem := rv.Elem(); elem.Kind() == reflect.Struct {
			s.fields(elem, path, callbacks)
			s.methods(rv, path, callbacks)
		} else {
			s.collect(elem, path, callbacks)
		}
	case reflect.Array, reflect.Slice:
		for i, v := 0, rv.Len(); i < v; i++ {
			s.collect(rv.Index(i), append(path, i), callbacks)
		}
	case reflect.Map:
		for _, mrv := range rv.MapKeys() {
			s.collect(rv.MapIndex(mrv), append(path, mrv.String()), callbacks)
		}
	case reflect.Struct:
		// register callback functions wrapper.
		if rv.Type() == dnodeFunctionType {
			if cb := rv.Interface().(Function); cb.Caller != nil {
				s.register(cb.Caller.(callback), path, callbacks)
			}
			return
		}
		s.fields(rv, path, callbacks)
		s.methods(rv, path, callbacks)
	case reflect.Func:
		panic("cannot marshal func, use Callback() to wrap it")
	}
}

// fields walks over a structure and scrubs its fields.
func (s *Scrubber) fields(rv reflect.Value, path Path, callbacks map[string]Path) {
	for i := 0; i < rv.NumField(); i++ {
		sf := rv.Type().Field(i)
		if sf.PkgPath != "" && !sf.Anonymous { // unexported.
			continue
		}

		// dnode uses JSON package tags for field naming so we need to
		// discard their comma-separated options.
		tag := sf.Tag.Get("json")
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		if tag == "-" {
			continue
		}
		// do not collect callbacks for "-" tagged fields.
		if skip := sf.Tag.Get("dnode"); skip == "-" {
			continue
		}

		var name = tag
		if name == "" {
			name = sf.Name
		}

		if sf.Anonymous {
			s.collect(rv.Field(i), path, callbacks)
		} else {
			s.collect(rv.Field(i), append(path, name), callbacks)
		}
	}
}

// methods walks over a structure and scrubs its exported methods.
func (s *Scrubber) methods(rv reflect.Value, path Path, callbacks map[string]Path) {
	for i := 0; i < rv.NumMethod(); i++ {
		if rv.Type().Method(i).PkgPath == "" { // exported
			cb, ok := rv.Method(i).Interface().(func(*Partial))
			if !ok {
				continue
			}

			name := rv.Type().Method(i).Name
			name = strings.ToLower(name[0:1]) + name[1:]
			s.register(cb, append(path, name), callbacks)
		}
	}
}

// register is called when a function/method is found in arguments array. It
// assigns an unique ID to the passed callback and stores it internally.
func (s *Scrubber) register(cb func(*Partial), path Path, callbacks map[string]Path) {
	// do not register nil callbacks.
	if cb == nil {
		return
	}
	// subtract one to start counting from zero. This is not absolutely
	// necessary, just cosmetics.
	next := atomic.AddUint64(&s.seq, 1) - 1
	seq := strconv.FormatUint(next, 10)

	// save in scubber callbacks.
	s.Lock()
	s.callbacks[next] = cb
	s.Unlock()

	// Add to callback map to be sent to remote. Make a copy of path because it
	// is reused in caller.
	pathCopy := make(Path, len(path))
	copy(pathCopy, path)
	callbacks[seq] = pathCopy
}
