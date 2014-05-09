package kite

import (
	"fmt"

	"github.com/koding/kite/dnode"
)

// Error is the type of the kite related errors returned from kite package.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e Error) Error() string {
	return fmt.Sprintf("kite error %s - %s - %s", e.Type, e.Message, e.Code)
}

// createError creates a new kite.Error for the given r variable
func createError(r interface{}) *Error {
	if r == nil {
		return nil
	}

	var kiteErr *Error
	switch err := r.(type) {
	case *Error:
		kiteErr = err
	case *dnode.ArgumentError:
		kiteErr = &Error{
			Type:    "argumentError",
			Message: err.Error(),
		}
	default:
		kiteErr = &Error{
			Type:    "genericError",
			Message: fmt.Sprint(r),
		}
	}

	return kiteErr
}
