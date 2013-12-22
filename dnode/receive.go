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
		msg     Message
		handler reflect.Value
		runner  Runner
	)

	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	// Get the handler function. Method may be string or integer.
	switch method := msg.Method.(type) {
	case float64:
		handler = d.callbacks[uint64(method)]
		runner = d.RunCallback
	case string:
		handler = d.handlers[method]
		runner = d.RunMethod
	default:
		return fmt.Errorf("Invalid method: %s", msg.Method)
	}

	// Method is not found.
	if handler == reflect.ValueOf(nil) {
		return fmt.Errorf("Unknown method: %v", msg.Method)
	}

	// Replace function placeholders with real functions.
	if err = d.parseCallbacks(&msg); err != nil {
		return err
	}

	// Must do this after parsing callbacks.
	var arguments []*Partial
	if err = msg.Arguments.Unmarshal(&arguments); err != nil {
		return err
	}

	if runner == nil {
		runner = defaultRunner
	}

	runner(fmt.Sprint(msg.Method), handler, Arguments(arguments), d.transport)
	return nil
}

func defaultRunner(method string, handlerFunc reflect.Value, args Arguments, tr Transport) {
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
