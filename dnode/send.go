package dnode

import (
	"encoding/json"
	"errors"
	"strconv"
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

	callbacks := d.scrubber.Scrub(arguments)

	defer func() {
		if err != nil {
			d.removeCallbacks(callbacks)
		}
	}()

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
	for sid, _ := range callbacks {
		// We don't check for error because we have created
		// the callbacks map in the send function above.
		// It does not come from remote, so cannot contain errors.
		id, _ := strconv.ParseUint(sid, 10, 64)
		d.scrubber.RemoveCallback(id)
	}
}
