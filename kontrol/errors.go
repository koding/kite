package kontrol

import (
	"bytes"
	"errors"
	"fmt"
)

var errNoSelfKeyPair = errors.New("kontrol has no key pair")

// ErrKeyDeleted is returned by Storage methods when
// requested key pair is no longer valid because it was
// deleted.
//
// The caller upon receiving this error may decide to
// recreate / resign a new kitekey or token for the caller
// (update the key).
var ErrKeyDeleted = errors.New("key pair is removed")

type multiError struct {
	err []error
}

func (me *multiError) Error() string {
	switch len(me.err) {
	case 0:
		return ""
	case 1:
		return me.err[0].Error()
	}

	var buf bytes.Buffer
	buf.WriteString("multiple errors occurred:\n\n")

	for _, err := range me.err {
		fmt.Fprintf(&buf, "  * %T: %s\n", err, err)
	}

	return buf.String()
}
