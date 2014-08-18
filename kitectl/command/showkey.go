package command

import (
	"fmt"
	"strings"

	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

var tokenKeyOrder = []string{
	"sub",        //  subject
	"iss",        // issuer
	"kontrolURL", // kontrol url
	"aud",        // audience
	"iat",        // issued at
	"jti",        // JWT ID
	"kontrolKey", // kontrol public key
}

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

	for _, v := range tokenKeyOrder {
		c.Ui.Output(fmt.Sprintf("%-15s%+v", v, token.Claims[v]))
	}

	return 0
}
