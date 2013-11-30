package dnode

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// processMessage processes a single message and call the previously
// added callbacks.
func (d *Dnode) processMessage(data []byte) error {
	l.Printf("processMessage: %s", string(data))

	var (
		err     error
		msg     Message
		handler Handler
	)

	defer func() {
		if err != nil {
			l.Printf("Cannot process message: %s", err)
		}
	}()

	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	if err = d.ParseCallbacks(&msg); err != nil {
		return err
	}

	// Get the handler function. Method may be string or integer.
	l.Printf("Received method: %s", msg.Method)
	switch method := msg.Method.(type) {
	case float64:
		handler = d.callbacks[uint64(method)]
	case string:
		handler = d.handlers[method]
	default:
		err = fmt.Errorf("Invalid method: %s", msg.Method)
		return err
	}

	// Method is not found.
	if handler == nil {
		err = fmt.Errorf("Unknown method: %v", msg.Method)
		return err
	}

	handler.ProcessMessage(&msg, d.transport)

	return nil
}

// ParseCallbacks parses the message's "callbacks" field and prepares
// callback functions in "arguments" field.
func (d *Dnode) ParseCallbacks(msg *Message) error {
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
			_, err := d.call(id, args...)
			return err
		})

		spec := CallbackSpec{path, f}
		msg.Arguments.CallbackSpecs = append(msg.Arguments.CallbackSpecs, spec)
	}

	return nil
}
