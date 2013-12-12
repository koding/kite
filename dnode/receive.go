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
	l.Printf("processMessage: %s", string(data))

	var (
		err     error
		msg     Message
		handler reflect.Value
		runner  Runner
	)

	defer func() {
		if err != nil {
			l.Printf("Cannot process message: %s", err)
		}
	}()

	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	// Get the handler function. Method may be string or integer.
	l.Printf("Received method: %s", fmt.Sprint(msg.Method))
	switch method := msg.Method.(type) {
	case float64:
		handler = d.callbacks[uint64(method)]
		runner = d.RunCallback
	case string:
		handler = d.handlers[method]
		runner = d.RunMethod
	default:
		err = fmt.Errorf("Invalid method: %s", msg.Method)
		return err
	}

	// Method is not found.
	if handler == reflect.ValueOf(nil) {
		err = fmt.Errorf("Unknown method: %v", msg.Method)
		return err
	}

	if err = d.parseCallbacks(&msg); err != nil {
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
	l.Printf("Received message callbacks: %#v", msg.Callbacks)

	for methodID, path := range msg.Callbacks {
		l.Printf("MehodID: %s", methodID)

		id, err := strconv.ParseUint(methodID, 10, 64)
		if err != nil {
			return err
		}

		// When the callback is called, we must send the method to the remote.
		f := Function(func(args ...interface{}) error {
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
