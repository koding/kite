package command

import (
	"encoding/json"
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

func toObject(v interface{}) (m map[string]interface{}) {
	p, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	if err := json.Unmarshal(p, &m); err != nil {
		panic(err)
	}

	return m
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

	claims := toObject(token.Claims)

	for _, v := range tokenKeyOrder {
		c.Ui.Output(fmt.Sprintf("%-15s%+v", v, claims[v]))
	}

	return 0
}
