package kite

import (
	"fmt"

	"github.com/koding/kite/dnode"
)

// MethodNotFoundError is returned when there is no registered handler for
// received method.
type MethodNotFoundError struct {
	Method string
	Args   *dnode.Partial
}

func (e MethodNotFoundError) Error() string {
	return fmt.Sprintf("Method not found: %s", e.Method)
}

// callbackNotFoundError is returned when there is no registered callback for
// received message.
type callbackNotFoundError struct {
	ID   uint64
	Args *dnode.Partial
}

func (e callbackNotFoundError) Error() string {
	return fmt.Sprintf("Callback ID not found: %d", e.ID)
}

// ArgumentError is returned when received message contains invalid arguments.
type ArgumentError struct {
	s string
}

func (e ArgumentError) Error() string {
	return e.s
}
