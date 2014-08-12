package command

import (
	"fmt"
	"strings"

	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type ShowkeyCommand struct {
	Ui cli.Ui
}

func (c *ShowkeyCommand) Synopsis() string {
	return "Shows the registration key"
}

func (c *ShowkeyCommand) Help() string {
	helpText := `
Usage: kitectl showkey

  Shows the registration key.
`
	return strings.TrimSpace(helpText)
}

func (c *ShowkeyCommand) Run(_ []string) int {
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
