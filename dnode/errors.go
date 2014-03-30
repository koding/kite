package dnode

import (
	"fmt"
)

// MethodNotFoundError is returned when there is no registered handler for
// received method.
type MethodNotFoundError struct {
	Method string
	Args   *Partial
}

func (e MethodNotFoundError) Error() string {
	return fmt.Sprintf("Method not found: %s", e.Method)
}

// CallbackNotFoundError is returned when there is no registered callback for
// received message.
type CallbackNotFoundError struct {
	ID   uint64
	Args *Partial
}

func (e CallbackNotFoundError) Error() string {
	return fmt.Sprintf("Callback ID not found: %d", e.ID)
}

// ArgumentError is returned when received message contains invalid arguments.
type ArgumentError struct {
	s string
}

func (e ArgumentError) Error() string {
	return e.s
}
