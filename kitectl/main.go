// Command line tool for using kite services.
package main

import (
	"fmt"
	"os"

	"github.com/koding/kite/kitectl/command"

	"github.com/mitchellh/cli"
)

func main() {
	c := cli.NewCLI(command.AppName, command.AppVersion)
	c.Args = os.Args[1:]
	c.Commands = map[string]cli.CommandFactory{
		"showkey":   command.NewShowkey(),
		"register":  command.NewRegister(),
		"query":     command.NewQuery(),
		"run":       command.NewRun(),
		"tell":      command.NewTell(),
		"uninstall": command.NewUninstall(),
		"list":      command.NewList(),
		"install":   command.NewInstall(),
	}

	_, err := c.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing CLI: %s\n", err.Error())
		return
	}
}
