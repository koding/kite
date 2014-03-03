// Kite is the command line tool for using kite services.
package main

import (
	"github.com/koding/kite"
	"github.com/koding/kite/cmd"
	"github.com/koding/kite/cmd/build"
	"github.com/koding/kite/cmd/cli"
)

const Version = "0.0.7"

func main() {
	client := kite.New("kite-cli", Version)

	root := cli.NewCLI()
	root.AddCommand("version", cmd.Version(Version))
	root.AddCommand("register", cmd.NewRegister(client))
	root.AddCommand("install", cmd.NewInstall())
	root.AddCommand("build", build.NewBuild())
	root.AddCommand("list", cmd.NewList())
	root.AddCommand("run", cmd.NewRun())
	root.AddCommand("tell", cmd.NewTell(client))
	root.AddCommand("uninstall", cmd.NewUninstall())
	root.AddCommand("showkey", cmd.NewShowKey())
	root.AddCommand("query", cmd.NewQuery(client))

	root.Run()
}
