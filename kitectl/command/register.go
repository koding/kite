package command

import (
	"flag"
	"strings"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

const defaultKontrolURL = "https://discovery.koding.com/kite"

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

  Registers your host to a kite authority.
  If no server is specified, "https://discovery.koding.io/kite" is the default.

Options:

  -to=https://discovery.koding.io/kite  Kontrol URL
  -username=koding                      Username
`
	return strings.TrimSpace(helpText)
}

func (c *Register) Run(args []string) int {
	var kontrolURL, username string
	var err error

	flags := flag.NewFlagSet("register", flag.ExitOnError)
	flags.StringVar(&kontrolURL, "to", defaultKontrolURL, "Kontrol URL")
	flags.StringVar(&username, "username", "", "Username")
	flags.Parse(args)

	// Open up a prompt
	if username == "" {
		username, err = c.Ui.Ask("Username:")
		if err != nil {
			c.Ui.Error(err.Error())
			return 1
		}
		// User can just press enter to use the default on the prompt
		if username == "" {
			c.Ui.Error("Username can not be empty.")
			return 1
		}
	}

	c.KiteClient.Config.Username = username

	if _, err := kitekey.Read(); err == nil {
		c.Ui.Info("Already registered. Registering again...")
	}

	kontrol := c.KiteClient.NewClient(kontrolURL)
	if err := kontrol.Dial(); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	result, err := kontrol.TellWithTimeout("registerMachine", 5*time.Minute, username)
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
