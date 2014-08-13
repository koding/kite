package command

import (
	"flag"
	"strconv"
	"strings"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type Tell struct {
	KiteClient *kite.Kite
	Ui         cli.Ui
}

func NewTell() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Tell{
			KiteClient: DefaultKiteClient,
			Ui:         DefaultUi,
		}, nil
	}
}

func (c *Tell) Synopsis() string {
	return "Calls a method on a kite"
}

func (c *Tell) Help() string {
	helpText := `
Usage: kitectl tell [options]

  Calls a method on a kite.

Options:

  -to=URL          URL of the remote kite
  -method=divide   Method name to be invoked
  -timeout=4       Timeout in seconds.
`
	return strings.TrimSpace(helpText)
}

func (c *Tell) Run(args []string) int {

	var to, method string
	var timeout time.Duration

	flags := flag.NewFlagSet("tell", flag.ExitOnError)
	flags.StringVar(&to, "to", "", "URL of remote kite")
	flags.StringVar(&method, "method", "", "method to be called")
	flags.DurationVar(&timeout, "timeout", 4*time.Second, "timeout of tell method")
	flags.Parse(args)

	if to == "" || method == "" {
		c.Ui.Output(c.Help())
		return 1
	}

	key, err := kitekey.Read()
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	remote := c.KiteClient.NewClient(to)
	remote.Auth = &kite.Auth{
		Type: "kiteKey",
		Key:  key,
	}

	if err = remote.Dial(); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// Convert args to []interface{} in order to pass it to Tell() method.
	methodArgs := flags.Args()
	params := make([]interface{}, len(methodArgs))
	for i, arg := range methodArgs {
		if number, err := strconv.Atoi(arg); err != nil {
			params[i] = arg
		} else {
			params[i] = number
		}
	}

	result, err := remote.TellWithTimeout(method, timeout, params...)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	if result == nil {
		c.Ui.Info("nil")
	} else {
		c.Ui.Info(string(result.Raw))
	}

	return 0
}
