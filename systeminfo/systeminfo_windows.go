package systeminfo

import (
	"errors"
)

// ErrNotImplemented - windows not supported/implemented
var ErrNotImplemented = errors.New("not implemented on windows")

func diskStats() (*disk, error) {
	return nil, ErrNotImplemented
}

func memoryStats() (*memory, error) {
	return nil, ErrNotImplemented
}
