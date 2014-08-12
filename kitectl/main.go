// Command line tool for using kite services.
package main

import (
	"github.com/koding/kite"
	"github.com/koding/kite/kitectl/cli"
	"github.com/koding/kite/kitectl/command"
)

const Version = "0.0.8"

func main() {
	client := kite.New("kite-cli", Version)

	c := cli.NewCLI()
	c.AddCommand("version", command.Version(Version))
	c.AddCommand("register", command.NewRegister(client))
	c.AddCommand("install", command.NewInstall())
	c.AddCommand("list", command.NewList())
	c.AddCommand("run", command.NewRun())
	c.AddCommand("tell", command.NewTell(client))
	c.AddCommand("uninstall", command.NewUninstall())
	c.AddCommand("showkey", command.NewShowKey())
	c.AddCommand("query", command.NewQuery(client))

	c.Run()
}
