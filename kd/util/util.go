// Package util contains the shared functions and constants for cli package.
package util

import (
	"os/user"
	"path/filepath"
)

// getKdPath returns absolute of ~/.kd
func GetKdPath() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return filepath.Join(usr.HomeDir, ".kd")
}
