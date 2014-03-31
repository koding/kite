package dnode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// processMessage processes a single message and call the previously
// added callbacks.
func (d *Dnode) processMessage(data []byte) error {
	var (
		err     error
		ok      bool
		msg     Message
		handler reflect.Value
		runner  Runner
	)

	// Call error handler.
	defer func() {
		if err != nil && d.OnError != nil {
			d.OnError(err)
		}
	}()

	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	// Replace function placeholders with real functions.
	if err = d.parseCallbacks(&msg); err != nil {
		return err
	}

	// Find the handler function. Method may be string or integer.
	switch method := msg.Method.(type) {
	case float64:
		id := uint64(method)
		runner = d.RunCallback
		if handler, ok = d.callbacks[id]; !ok {
			err = CallbackNotFoundError{id, msg.Arguments}
			return err
		}
	case string:
		runner = d.RunMethod
		if handler, ok = d.handlers[method]; !ok {
			err = MethodNotFoundError{method, msg.Arguments}
			return err
		}
	default:
		err = fmt.Errorf("Mehtod is not string or integer: %q", msg.Method)
		return err
	}

	if runner == nil {
		runner = defaultRunner
	}

	runner(fmt.Sprint(msg.Method), handler, msg.Arguments, d.transport)

	return nil
}

func defaultRunner(method string, handlerFunc reflect.Value, args *Partial, tr Transport) {
	// Call the handler with arguments.
	callArgs := []reflect.Value{reflect.ValueOf(args)}
	handlerFunc.Call(callArgs)
}

// parseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func (d *Dnode) parseCallbacks(msg *Message) error {
	// Parse callbacks field and create callback functions.
	for methodID, path := range msg.Callbacks {
		id, err := strconv.ParseUint(methodID, 10, 64)
		if err != nil {
			return err
		}

		// When the callback is called, we must send the method to the remote.
		f := Function(func(args ...interface{}) error {
			if args == nil {
				args = make([]interface{}, 0)
			}
			if d.WrapCallbackArgs != nil {
				args = d.WrapCallbackArgs(args, d.transport)
			}

			_, err := d.send(id, args)
			return err
		})

		spec := CallbackSpec{path, f}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}

	return nil
}
