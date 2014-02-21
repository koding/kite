package cmd

import (
	"flag"
	"fmt"
	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
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
	var to, method string

	flags := flag.NewFlagSet("tell", flag.ExitOnError)
	flags.StringVar(&to, "to", "", "URL of remote kite")
	flags.StringVar(&method, "method", "", "method to be called")
	flags.Parse(args)

	parsed, err := url.Parse(to)
	if err != nil {
		return err
	}

	key, err := kitekey.Read()
	if err != nil {
		return err
	}

	auth := kite.Authentication{
		Type: "kiteKey",
		Key:  key,
	}

	remote := t.client.NewRemoteKite(protocol.Kite{URL: &protocol.KiteURL{*parsed}}, auth)

	if err = remote.Dial(); err != nil {
		return err
	}

	// Convert args to []interface{} in order to pass it to Tell() method.
	methodArgs := flags.Args()
	params := make([]interface{}, len(methodArgs))
	for i, arg := range methodArgs {
		if number, err := strconv.Atoi(arg); err != nil {
			params[i] = arg
		} else {
			params[i] = number
		}
	}

	result, err := remote.Tell(method, params...)
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
