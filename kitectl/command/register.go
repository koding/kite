package command

import (
	"flag"
	"strings"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type Register struct {
	KiteClient *kite.Kite
	Ui         cli.Ui
}

func NewRegister() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Register{
			KiteClient: DefaultKiteClient,
			Ui:         DefaultUi,
		}, nil
	}
}

func (c *Register) Synopsis() string {
	return "Registers this host to a kite authority"
}

func (c *Register) Help() string {
	helpText := `
Usage: kitectl register [options]

  Registers this host to a kite authority.

Options:

  -to=http://127.0.0.1  Kontrol URL.
  -username=koding
`
	return strings.TrimSpace(helpText)
}

func (c *Register) Run(args []string) int {
	flags := flag.NewFlagSet("register", flag.ExitOnError)
	to := flags.String("to", "", "target registration server")
	username := flags.String("username", "", "pick a username")
	flags.Parse(args)

	if *to == "" || *username == "" {
		c.Ui.Output(c.Help())
		return 1
	}

	c.KiteClient.Config.Username = *username

	if _, err := kitekey.Read(); err == nil {
		c.Ui.Info("Already registered. Registering again...")
	}

	kontrol := c.KiteClient.NewClient(*to)
	if err := kontrol.Dial(); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	result, err := kontrol.TellWithTimeout("registerMachine", 5*time.Minute, *username)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	if err := kitekey.Write(result.MustString()); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	c.Ui.Info("Registered successfully")

	return 0
}
