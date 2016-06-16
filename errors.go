package kite

import (
	"errors"
	"fmt"

	"github.com/koding/kite/dnode"
)

// ErrKeyNotTrusted is returned by verify functions when the key
// should not be trusted.
var ErrKeyNotTrusted = errors.New("kontrol key is not trusted")

// Error is the type of the kite related errors returned from kite package.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	CodeVal string `json:"code"`
}

func (e Error) Code() string {
	return e.CodeVal
}

func (e Error) Error() string {
	if e.Type == "genericError" || e.Type == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Type, e.Message)
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
