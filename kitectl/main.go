// Command line tool for using kite services.
package main

import (
	"fmt"
	"os"

	"github.com/koding/kite"
	"github.com/koding/kite/kitectl/command"

	"github.com/mitchellh/cli"
)

const (
	name    = "kitectl"
	version = "0.0.8"
)

func main() {
	c := cli.CLI{
		Name:     name,
		Version:  version,
		Args:     os.Args[1:],
		Commands: commands,
		HelpFunc: cli.BasicHelpFunc(name),
	}

	_, err := c.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing CLI: %s\n", err.Error())
		return
	}
}

var commands map[string]cli.CommandFactory

func init() {
	kiteClient := kite.New(name, version)

	ui := &cli.ColoredUi{
		InfoColor:  cli.UiColorYellow,
		ErrorColor: cli.UiColorRed,
		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stdout,
		},
	}

	commands = map[string]cli.CommandFactory{
		"version": func() (cli.Command, error) {
			return &command.VersionCommand{
				Version: version,
				Ui:      ui,
			}, nil
		},
		"showkey": func() (cli.Command, error) {
			return &command.ShowkeyCommand{
				Ui: ui,
			}, nil
		},
		"register": func() (cli.Command, error) {
			return &command.RegisterCommand{
				KiteClient: kiteClient,
				Ui:         ui,
			}, nil
		},
		"query": func() (cli.Command, error) {
			return &command.QueryCommand{
				KiteClient: kiteClient,
				Ui:         ui,
			}, nil
		},
		"run": func() (cli.Command, error) {
			return &command.RunCommand{
				Ui: ui,
			}, nil
		},
		"tell": func() (cli.Command, error) {
			return &command.TellCommand{
				KiteClient: kiteClient,
				Ui:         ui,
			}, nil
		},
		"uninstall": func() (cli.Command, error) {
			return &command.UninstallCommand{
				Ui: ui,
			}, nil
		},
		"list": func() (cli.Command, error) {
			return &command.ListCommand{
				Ui: ui,
			}, nil
		},
		"install": func() (cli.Command, error) {
			return &command.InstallCommand{
				Ui: ui,
			}, nil
		},
	}
}
