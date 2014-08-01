package cmd

import (
	"flag"
	"fmt"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
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

	if _, err := kitekey.Read(); err == nil {
		r.client.Log.Warning("Already registered. Registering again...")
	}

	kontrol := r.client.NewClient(*to)
	if err := kontrol.Dial(); err != nil {
		return err
	}

	result, err := kontrol.TellWithTimeout("registerMachine", 5*time.Minute, *username)
	if err != nil {
		return err
	}

	if err := kitekey.Write(result.MustString()); err != nil {
		return err
	}

	fmt.Println("Registered successfully")
	return nil
}
