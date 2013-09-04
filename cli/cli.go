package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	uuid "github.com/nu7hatch/gouuid"
	"io/ioutil"
	"net/http"
	"os/user"
	"strings"
	"time"
)

// var authSite = "https://koding.com"

var authSite = "http://localhost:3020"

type Kd struct {
	ID         string
	PublicKey  string
	PrivateKey string
	Username   string
}

func NewKd() *Kd {
	id, _ := uuid.NewV4()
	return &Kd{
		ID: "machineID-" + id.String(),
	}
}

func main() {
	k := NewKd()
	err := k.Register()
	if err != nil {
		fmt.Printf("\nregister error: %s\n", err.Error())
	}

}

func (k *Kd) Register() error {
	err := k.GetConfig()
	if err != nil {
		return err
	}

	registerUrl := fmt.Sprintf("%s/-/auth/register/%s/%s", authSite, k.ID, k.PublicKey)
	checkUrl := fmt.Sprintf("%s/-/auth/check/%s", authSite, k.PublicKey)

	fmt.Printf("Please open the following url for authentication:")
	PrintBox(registerUrl)
	fmt.Printf("waiting . ")

	// check the result every two seconds
	ticker := time.NewTicker(time.Second * 2).C

	// wait for three minutes, if not successfull abort it
	timeout := time.After(time.Minute * 3)

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

	return nil
}

func PrintBox(msg string) {
	fmt.Printf("\n\n")
	fmt.Println("╔" + strings.Repeat("═", len(msg)+2) + "╗")
	fmt.Println("║" + strings.Repeat(" ", len(msg)+2) + "║")
	fmt.Println("║ " + msg + " ║")
	fmt.Println("║" + strings.Repeat(" ", len(msg)+2) + "║")
	fmt.Println("╚" + strings.Repeat("═", len(msg)+2) + "╝")
	fmt.Printf("\n")
}

func (k *Kd) GetConfig() error {
	var err error
	// we except to read all of them, if one fails it should be abort
	k.PublicKey, err = GetKey("public")
	if err != nil {
		return err
	}

	k.PrivateKey, err = GetKey("private")
	if err != nil {
		return err
	}
	return nil
}

func GetKey(key string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	var keyfile string
	switch key {
	case "public":
		keyfile = usr.HomeDir + "/.kd/koding.key.pub"
	case "private":
		keyfile = usr.HomeDir + "/.kd/koding.key"
	default:
		return "", fmt.Errorf("key is not recognized '%s'\n", key)
	}

	file, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(file)), nil
}
