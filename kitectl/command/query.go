package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/protocol"
	"github.com/mitchellh/cli"
)

type Query struct {
	KiteClient *kite.Kite
	Ui         cli.Ui
}

func NewQuery() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Query{
			KiteClient: DefaultKiteClient,
			Ui:         DefaultUi,
		}, nil
	}
}

func (c *Query) Synopsis() string {
	return "Queries kontrol based on the given criteria"
}

func (c *Query) Help() string {
	helpText := `
Usage: kitectl query [options]

  Queries Kontrol based on the given criteria.

Options:

  -username=koding      Username of the kite.
  -environment=staging  Environment of the kite.
  -name=naber           Name of the kite.
  -version=0.0.1        Version of the kite.
  -region=Asia          Region of the kite.
  -hostname=caprica     Hostname of the kite.
  -id=<UUID>            Unique ID of the kite.
`
	return strings.TrimSpace(helpText)
}

func (c *Query) Run(args []string) int {
	c.KiteClient.Config = config.MustGet()
	c.KiteClient.Config.Transport = config.XHRPolling

	var query protocol.KontrolQuery

	flags := flag.NewFlagSet("query", flag.ExitOnError)
	flags.StringVar(&query.Username, "username", c.KiteClient.Kite().Username, "")
	flags.StringVar(&query.Environment, "environment", "", "")
	flags.StringVar(&query.Name, "name", "", "")
	flags.StringVar(&query.Version, "version", "", "")
	flags.StringVar(&query.Region, "region", "", "")
	flags.StringVar(&query.Hostname, "hostname", "", "")
	flags.StringVar(&query.ID, "id", "", "")
	flags.Parse(args)

	result, err := c.KiteClient.GetKites(&query)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	for i, client := range result {
		var k *protocol.Kite = &client.Kite
		c.Ui.Output(fmt.Sprintf(
			"%d\t%s/%s/%s/%s/%s/%s/%s\t%s",
			i+1,
			k.Username,
			k.Environment,
			k.Name,
			k.Version,
			k.Region,
			k.Hostname,
			k.ID,
			client.URL,
		))
	}

	return 0
}
