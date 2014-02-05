package main

import (
	"kite"
	"kite/cmd"
	"kite/cmd/build"
	"kite/cmd/cli"
)

func main() {
	options := &kite.Options{
		Kitename:    "kite-command",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "production",
	}
	client := kite.New(options)
	client.KontrolEnabled = false

	root := cli.NewCLI()
	root.AddCommand("version", cmd.NewVersion())
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
