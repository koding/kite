package main

import (
	"koding/newkite/kd"
	"koding/newkite/kd/cli"
	"koding/newkite/kd/kite"
)

func main() {
	root := cli.NewCLI()
	root.AddCommand("version", kd.NewVersion())
	root.AddCommand("register", kd.NewRegister())

	k := root.AddSubCommand("kite")
	k.AddCommand("install", kite.NewInstall())
	k.AddCommand("list", kite.NewList())
	k.AddCommand("uninstall", kite.NewUninstall())

	root.Run()
}
