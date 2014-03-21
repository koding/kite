package systeminfo

import (
	"errors"
)

var errNotImplemented = errors.New("not implemented on windows")

func diskStats() (*disk, error) {
	return nil, errNotImplemented
}

func memoryStats() (*memory, error) {
	return nil, errNotImplemented
}
