package dnode

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// processMessage processes a single message and call the previously
// added callbacks.
func (d *Dnode) processMessage(data []byte) error {
	var (
		err     error
		ok      bool
		msg     Message
		handler func(*Partial)
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
		if handler, ok = d.scrubber.callbacks[id]; !ok {
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

func defaultRunner(method string, handlerFunc func(*Partial), args *Partial, tr Transport) {
	handlerFunc(args)
}

// parseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func (d *Dnode) parseCallbacks(msg *Message) error {
	return ParseCallbacks(msg, func(id uint64, args []interface{}) error {
		if args == nil {
			args = make([]interface{}, 0)
		}
		if d.WrapCallbackArgs != nil {
			args = d.WrapCallbackArgs(args, d.transport)
		}

		_, err := d.send(id, args)
		return err
	})
}

// parseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func ParseCallbacks(msg *Message, sender func(id uint64, args []interface{}) error) error {
	// Parse callbacks field and create callback functions.
	for methodID, path := range msg.Callbacks {
		id, err := strconv.ParseUint(methodID, 10, 64)
		if err != nil {
			return err
		}

		f := func(args ...interface{}) error { return sender(id, args) }
		spec := CallbackSpec{path, Function{functionReceived(f)}}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}

	return nil
}
