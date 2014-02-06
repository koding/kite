package cmd

import (
	"errors"
	"fmt"
	"kite"
	"kite/kitekey"
	"kite/protocol"
	"net/url"
	"strconv"
)

type Tell struct {
	client *kite.Kite
}

func NewTell(client *kite.Kite) *Tell {
	return &Tell{
		client: client,
	}
}

func (t *Tell) Definition() string {
	return "Call a method on a kite"
}

func (t *Tell) Exec(args []string) error {
	if len(args) < 2 {
		return errors.New("You must give a URL, method and arguments, all seperated by space")
	}

	parsed, err := url.Parse(args[0])
	if err != nil {
		return err
	}

	target := protocol.Kite{URL: protocol.KiteURL{parsed}}

	kodingKey, err := kitekey.Read()
	if err != nil {
		return err
	}

	auth := kite.Authentication{
		Type: "kodingKey",
		Key:  kodingKey,
	}

	remote := t.client.NewRemoteKite(target, auth)

	if err = remote.Dial(); err != nil {
		return err
	}

	// Convert args to []interface{} in order to pass it to Tell() method.
	methodArgs := args[2:]
	params := make([]interface{}, len(methodArgs))
	for i, arg := range methodArgs {
		if number, err := strconv.Atoi(arg); err != nil {
			params[i] = arg
		} else {
			params[i] = number
		}
	}

	result, err := remote.Tell(args[1], params...)
	if err != nil {
		return err
	}

	if result == nil {
		fmt.Println("nil")
	} else {
		fmt.Println(string(result.Raw))
	}

	return nil
}
