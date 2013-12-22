package kite

import (
	"errors"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"net/url"
	"strconv"
)

type Tell struct{}

func NewTell() *Tell {
	return &Tell{}
}

func (*Tell) Definition() string {
	return "Call a method on a Kite"
}

func (*Tell) Exec(args []string) error {
	if len(args) < 2 {
		return errors.New("You must give a URL, method and arguments, all seperated by space")
	}

	parsed, err := url.Parse(args[0])
	if err != nil {
		return err
	}

	options := &kite.Options{
		Kitename:    "kd-tool",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "production",
	}

	k := kite.New(options)

	target := protocol.Kite{URL: protocol.KiteURL{parsed}}

	kodingKey, err := utils.GetKodingKey()
	if err != nil {
		return err
	}

	auth := kite.Authentication{
		Type: "kodingKey",
		Key:  kodingKey,
	}

	remote := k.NewRemoteKite(target, auth)

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
