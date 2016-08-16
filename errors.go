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
	Type      string `json:"type"`
	Message   string `json:"message"`
	CodeVal   string `json:"code"`
	RequestID string `json:"id"`
}

func (e Error) Code() string {
	return e.CodeVal
}

func (e Error) Error() string {
	s := e.Message

	if e.Type != "genericError" && e.Type != "" {
		s = e.Type + ": " + e.Message
	}

	if e.RequestID != "" {
		return s + " (" + e.RequestID + ")"
	}

	return s
}

// createError creates a new kite.Error for the given r variable
func createError(req *Request, r interface{}) *Error {
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

	if kiteErr.RequestID == "" && req != nil {
		kiteErr.RequestID = req.ID
	}

	return kiteErr
}
