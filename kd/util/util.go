// Package util contains the shared functions and constants for cli package.
package util

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/nu7hatch/gouuid"
)

var KeyPath = filepath.Join(GetKdPath(), "koding.key")

// getKdPath returns absolute of ~/.kd
func GetKdPath() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return filepath.Join(usr.HomeDir, ".kd")
}

// getKey returns the Koding key content from ~/.kd/koding.key
func GetKey() (string, error) {
	data, err := ioutil.ReadFile(KeyPath)
	if err != nil {
		return "", err
	}

	key := strings.TrimSpace(string(data))

	return key, nil
}

// writeKey writes the content of the given key to ~/.kd/koding.key
func WriteKey(key string) error {
	os.Mkdir(GetKdPath(), 0700) // create if not exists

	err := ioutil.WriteFile(KeyPath, []byte(key), 0600)
	if err != nil {
		return err
	}

	return nil
}

// hostID returns a unique string that defines a machine
func HostID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	return hostname + "-" + id.String(), nil
}
