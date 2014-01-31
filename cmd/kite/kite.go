package main

import (
	"koding/kite/cmd"
	"koding/kite/cmd/build"
	"koding/kite/cmd/cli"
)

func main() {
	root := cli.NewCLI()

	root.AddCommand("version", cmd.NewVersion())
	root.AddCommand("register", cmd.NewRegister())
	root.AddCommand("install", cmd.NewInstall())
	root.AddCommand("build", build.NewBuild())
	root.AddCommand("list", cmd.NewList())
	root.AddCommand("run", cmd.NewRun())
	root.AddCommand("tell", cmd.NewTell())
	root.AddCommand("uninstall", cmd.NewUninstall())

	root.Run()
}
