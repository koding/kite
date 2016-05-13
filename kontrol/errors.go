package kontrol

import (
	"bytes"
	"errors"
	"fmt"
)

var errNoSelfKeyPair = errors.New("kontrol has no key pair")

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
