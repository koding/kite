package command

import (
	"fmt"
	"strings"

	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type Showkey struct {
	Ui cli.Ui
}

func NewShowkey() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Showkey{
			Ui: DefaultUi,
		}, nil
	}
}

func (c *Showkey) Synopsis() string {
	return "Shows the registration key"
}

func (c *Showkey) Help() string {
	helpText := `
Usage: kitectl showkey

  Shows the registration key.
`
	return strings.TrimSpace(helpText)
}

func (c *Showkey) Run(_ []string) int {
	token, err := kitekey.Parse()
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	for k, v := range token.Claims {
		c.Ui.Output(fmt.Sprintf("%-15s%+v", k, v))
	}

	return 0
}
