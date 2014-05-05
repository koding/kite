package kite

import (
	"fmt"
	"runtime/debug"

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

// recoverError returns a function which recovers the error and sets to the
// given argument as kite.Error.
//
// TODO: change that it doesn't use a pointer of pointer,
// a simpler and cleaner solution would work in the future,
func (k *Kite) recoverError(kiteErr **Error) func() {
	return func() {
		r := recover()
		if r == nil {
			return
		}

		switch err := r.(type) {
		case *Error:
			*kiteErr = err
		case *dnode.ArgumentError:
			*kiteErr = &Error{
				Type:    "argumentError",
				Message: err.Error(),
			}
		default:
			*kiteErr = &Error{
				Type:    "genericError",
				Message: fmt.Sprint(r),
			}
			debug.PrintStack()
		}

		k.Log.Warning("Error in received message: %s", (*kiteErr).Error())

	}
}
