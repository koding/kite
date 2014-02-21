package cmd

import (
	"flag"
	"fmt"
	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"net/url"
	"os"
	"time"
)

const defaultRegServ = "ws://localhost:8080/regserv"

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
	to := flags.String("to", defaultRegServ, "target registration server")
	flags.Parse(args)

	_, err := kitekey.Read()
	if err == nil {
		r.client.Log.Warning("Already registered. Registering again...")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	parsed, err := url.Parse(*to)
	if err != nil {
		return err
	}

	target := protocol.Kite{URL: &protocol.KiteURL{*parsed}}
	regserv := r.client.NewRemoteKite(target, kite.Authentication{})
	if err = regserv.Dial(); err != nil {
		return err
	}

	result, err := regserv.TellWithTimeout("register", 10*time.Minute, map[string]string{"hostname": hostname})
	if err != nil {
		return err
	}

	err = kitekey.Write(result.MustString())
	if err != nil {
		return err
	}

	fmt.Println("Registered successfully")
	return nil
}
