package kd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	uuid "github.com/nu7hatch/gouuid"
	"io/ioutil"
	"koding/newkite/kd/util"
	"koding/newkite/kodingkey"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const KeyLength = 64

var AuthServer = "https://koding.com"

// TODO change this before deploying to production
// var AuthServer = "http://localhost:3020"

type Register struct{}

func NewRegister() *Register {
	return &Register{}
}

func (r *Register) Definition() string {
	return "Register this host to Koding"
}

func (r *Register) Exec(args []string) error {
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	hostID := hostname + "-" + id.String()

	key, err := getOrCreateKey()
	if err != nil {
		return err
	}

	registerUrl := fmt.Sprintf("%s/-/auth/register/%s/%s", AuthServer, hostID, key)

	fmt.Printf("Please open the following url for authentication:\n\n")
	fmt.Println(registerUrl)
	fmt.Printf("\nwaiting . ")

	return checker(key)
}

// checker checks if the user has browsed the register URL by polling the check URL.
func checker(key string) error {
	checkUrl := fmt.Sprintf("%s/-/auth/check/%s", AuthServer, key)

	// check the result every two seconds
	ticker := time.NewTicker(2 * time.Second).C

	// wait for three minutes, if not successfull abort it
	timeout := time.After(3 * time.Minute)

	for {
		select {
		case <-ticker:
			resp, err := http.Get(checkUrl)
			if err != nil {
				return err
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			resp.Body.Close()
			fmt.Printf(". ")

			if resp.StatusCode == 200 {
				type Result struct {
					Result string `json:"result"`
				}

				res := Result{}

				err := json.Unmarshal(bytes.TrimSpace(body), &res)
				if err != nil {
					return err
				}

				fmt.Println(res.Result)
				return nil
			}
		case <-timeout:
			return errors.New("timeout")
		}
	}
}

// getOrCreateKey combines the two functions: getKey and writeNewKey
func getOrCreateKey() (string, error) {
	kdPath := util.GetKdPath()
	keyPath := filepath.Join(kdPath, "koding.key")
	key, err := getKey(keyPath)
	if err == nil {
		return key, nil
	}

	if !os.IsNotExist(err) {
		return "", err
	}

	key, err = writeNewKey(kdPath, keyPath)
	if err != nil {
		return "", err
	}

	return key, nil

}

// getKey returns the Koding key from ~/.kd/koding.key
func getKey(keyPath string) (string, error) {
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return "", err
	}

	key := strings.TrimSpace(string(data))

	return key, nil
}

// writeNewKey generates a new Koding key and writes to ~/.kd/koding.key
func writeNewKey(kdPath, keyPath string) (string, error) {
	fmt.Println("Koding key is not found on this host. A new key will be created.")

	err := os.Mkdir(kdPath, 0700)

	key, err := kodingkey.NewKodingKey()
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(keyPath, []byte(key.String()), 0600)
	if err != nil {
		return "", err
	}

	return key.String(), nil
}
