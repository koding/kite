package cmd

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
)

const defaultRegServ = "ws://localhost:3998/regserv"

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
	flags := flag.NewFlagSet("register", flag.ExitOnError)
	to := flags.String("to", "", "target registration server")
	username := flags.String("username", "", "pick a username")
	flags.Parse(args)

	if *to == "" {
		r.client.Log.Fatal("No URL given in -to flag")
	}

	if *username == "" {
		r.client.Log.Fatal("You must give a username with -username flag")
	}
	r.client.Config.Username = *username

	parsed, err := url.Parse(*to)
	if err != nil {
		return err
	}

	if _, err = kitekey.Read(); err == nil {
		r.client.Log.Warning("Already registered. Registering again...")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	regserv := r.client.NewClient(parsed)
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
