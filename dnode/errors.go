package dnode

import (
	"fmt"
)

// MethodNotFoundError is returned when there is no registered handler for
// received method.
type MethodNotFoundError struct {
	Method string
	Args   Arguments
}

func (e MethodNotFoundError) Error() string {
	return fmt.Sprint("Unknown method: %s", e.Method)
}

// CallbackNotFoundError is returned when there is no registered callback for
// received message.
type CallbackNotFoundError struct {
	ID   uint64
	Args Arguments
}

func (e CallbackNotFoundError) Error() string {
	return fmt.Sprint("Unknown callback ID: %d", e.ID)
}

// ArgumentError is returned when received message contains invalid arguments.
type ArgumentError struct {
	s string
}

func (e ArgumentError) Error() string {
	return e.s
}
