// Package util contains the shared functions and constants for cli package.
package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/nu7hatch/gouuid"
)

var KeyPath = filepath.Join(GetKdPath(), "koding.key")

const (
	AuthServer      = "https://koding.com"
	AuthServerLocal = "http://localhost:3020"
)

// getKdPath returns absolute of ~/.kd
func GetKdPath() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return filepath.Join(usr.HomeDir, ".kd")
}

// GetKey returns the Koding key content from ~/.kd/koding.key
func GetKey() (string, error) {
	data, err := ioutil.ReadFile(KeyPath)
	if err != nil {
		return "", err
	}

	key := strings.TrimSpace(string(data))

	return key, nil
}

// WriteKey writes the content of the given key to ~/.kd/koding.key
func WriteKey(key string) error {
	os.Mkdir(GetKdPath(), 0700) // create if not exists

	err := ioutil.WriteFile(KeyPath, []byte(key), 0600)
	if err != nil {
		return err
	}

	return nil
}

// CheckKey checks wether the key is registerd to koding, or not
func CheckKey(authServer, key string) error {
	checkUrl := CheckURL(authServer, key)

	resp, err := http.Get(checkUrl)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("non 200 response")
	}

	type Result struct {
		Result string `json:"result"`
	}

	res := Result{}
	err = json.Unmarshal(bytes.TrimSpace(body), &res)
	if err != nil {
		log.Fatalln(err) // this should not happen, exit here
	}

	return nil
}

func CheckURL(authServer, key string) string {
	return fmt.Sprintf("%s/-/auth/check/%s", authServer, key)
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
