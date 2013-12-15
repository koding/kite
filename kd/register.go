package kd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"koding/newkite/kd/util"
	"koding/newkite/kodingkey"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	uuid "github.com/nu7hatch/gouuid"
)

const KeyLength = 64

var (
	AuthServer      = "https://koding.com"
	AuthServerLocal = "http://localhost:3020"
	KdPath          = util.GetKdPath()
	KeyPath         = filepath.Join(KdPath, "koding.key")
)

type Register struct {
	authServer string
}

func NewRegister() *Register {
	return &Register{
		authServer: AuthServer,
	}
}

func (r *Register) Definition() string {
	return "Register this host to Koding"
}

func (r *Register) Exec(args []string) error {
	// change authServer address if debug mode is enabled
	if len(args) == 1 && (args[0] == "--debug" || args[0] == "-d") {
		r.authServer = AuthServerLocal
	}

	hostID, err := hostID()
	if err != nil {
		return err
	}

	var key string
	keyExist := false

	key, err = getKey()
	if err != nil {
		k, err := kodingkey.NewKodingKey()
		if err != nil {
			return err
		}

		key = k.String()
	} else {
		fmt.Printf("Found a key under '%s'. Going to use it to register\n", KdPath)
		keyExist = true
	}

	registerUrl := fmt.Sprintf("%s/-/auth/register/%s/%s", r.authServer, hostID, key)

	fmt.Printf("Please open the following url for authentication:\n\n")
	fmt.Println(registerUrl)
	fmt.Printf("\nwaiting . ")

	err = r.checker(key)
	if err != nil {
		return err
	}
	fmt.Println("successfully authenticated.")

	if keyExist {
		return nil
	}

	err = writeKey(key)
	if err != nil {
		return err
	}

	return nil
}

// checker checks if the user has browsed the register URL by polling the check URL.
func (r *Register) checker(key string) error {
	checkUrl := fmt.Sprintf("%s/-/auth/check/%s", r.authServer, key)

	// check the result every two seconds
	ticker := time.NewTicker(2 * time.Second).C

	// wait for three minutes, if not successfull abort it
	timeout := time.After(3 * time.Minute)

	for {
		select {
		case <-ticker:
			err := checkResponse(checkUrl)
			if err != nil {
				// we didn't get OK message, continue until timout
				continue
			}

			return nil
		case <-timeout:
			return errors.New("timeout")
		}
	}
}

func checkResponse(checkUrl string) error {
	resp, err := http.Get(checkUrl)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	fmt.Printf(". ") // animation

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

// getKey returns the Koding key content from ~/.kd/koding.key
func getKey() (string, error) {
	data, err := ioutil.ReadFile(KeyPath)
	if err != nil {
		return "", err
	}

	key := strings.TrimSpace(string(data))

	return key, nil
}

// writeKey writes the content of the given key to ~/.kd/koding.key
func writeKey(key string) error {
	os.Mkdir(KdPath, 0700) // create if not exists

	err := ioutil.WriteFile(KeyPath, []byte(key), 0600)
	if err != nil {
		return err
	}

	return nil
}

// hostID returns a unique string that defines a machine
func hostID() (string, error) {
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
