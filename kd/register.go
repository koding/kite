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
	"time"
)

const KeyLength = 64

var (
	AuthServer      = "https://koding.com"
	AuthServerLocal = "http://localhost:3020"
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

	hostID, err := util.HostID()
	if err != nil {
		return err
	}

	var key string
	keyExist := false

	key, err = util.GetKey()
	if err != nil {
		k, err := kodingkey.NewKodingKey()
		if err != nil {
			return err
		}

		key = k.String()
	} else {
		fmt.Printf("Found a key under '%s'. Going to use it to register\n", util.GetKdPath())
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

	err = util.WriteKey(key)
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
