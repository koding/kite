package cmd

import (
	"errors"
	"flag"
	"fmt"
	"kite"
	"kite/cmd/util"
	"kite/protocol"
	"net/url"
	"os"
)

type Register struct {
	client *kite.Kite
}

func NewRegister(client *kite.Kite) *Register {
	return &Register{
		client: client,
	}
}

func (r *Register) Definition() string {
	return "Register this host to a kite authority"
}

func (r *Register) Exec(args []string) error {
	flags := flag.NewFlagSet("register", flag.ContinueOnError)
	to := flags.String("to", "ws://localhost:8080/regserv", "target registration server")
	flags.Parse(args)

	key, err := util.KiteKey()
	if err == nil && key != "" {
		return errors.New("Already registered")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	parsed, err := url.Parse(*to)
	if err != nil {
		return err
	}

	target := protocol.Kite{URL: protocol.KiteURL{parsed}}
	regserv := r.client.NewRemoteKite(target, kite.Authentication{})
	if err = regserv.Dial(); err != nil {
		return err
	}

	result, err := regserv.Tell("register", map[string]string{"hostname": hostname})
	if err != nil {
		return err
	}

	var keys struct {
		KiteKey    string `json:"kite.key"`
		KontrolKey string `json:"kontrol.key"`
	}

	err = result.Unmarshal(&keys)
	if err != nil {
		return err
	}

	err = util.WriteKeys(keys.KiteKey, keys.KontrolKey)
	if err != nil {
		return err
	}

	fmt.Println("Registered successfully")
	return nil
}
