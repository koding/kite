package kd

import (
	"errors"
	"fmt"
	"koding/newkite/kd/util"
	"koding/newkite/kodingkey"
	"time"
)

const KeyLength = 64

type Register struct{}

func NewRegister() *Register {
	return &Register{}
}

func (r *Register) Definition() string {
	return "Register this host to Koding"
}

func (r *Register) Exec(args []string) error {
	authServer := util.AuthServer

	// change authServer address if debug mode is enabled
	if len(args) == 1 && (args[0] == "--debug" || args[0] == "-d") {
		authServer = util.AuthServerLocal
	}

	// i.e: kd register to latest.koding.com
	//  	kd register to localhost:4000
	if len(args) == 2 && args[0] == "to" {
		authServer = fmt.Sprintf("http://%s", args[1])
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

	registerUrl := fmt.Sprintf("%s/-/auth/register/%s/%s", authServer, hostID, key)

	// first check if the user is alrady registered
	err = util.CheckKey(authServer, key)
	if err == nil {
		fmt.Printf("... you are already registered.\n")
		return nil
	}

	fmt.Printf("Please open the following url for authentication:\n\n")
	fmt.Println(registerUrl)
	fmt.Printf("\nwaiting . ")

	// .. if not let the user register himself
	err = checker(authServer, key)
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

// checker checks if the user has browsed the register URL by polling the
// check URL.
func checker(authServer, key string) error {
	// check the result every two seconds
	ticker := time.NewTicker(2 * time.Second).C

	// wait for three minutes, if not successfull abort it
	timeout := time.After(3 * time.Minute)

	for {
		select {
		case <-ticker:
			err := util.CheckKey(authServer, key)
			if err != nil {
				// we didn't get OK message, continue until timout
				fmt.Printf(". ") // animation
				continue
			}

			return nil
		case <-timeout:
			return errors.New("timeout")
		}
	}
}
