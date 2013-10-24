package cli

import (
	"flag"
	"koding/newKite/kd/cli/kite"
)

type Dispatcher struct {
	root *Module
}

func NewDispatcher() *Dispatcher {
	root := NewModule(nil, "")
	root.AddCommand("version", NewVersion())
	root.AddCommand("register", NewRegister())

	k := root.AddModule("kite", "Includes commands related to kites")
	k.AddCommand("install", kite.NewInstall())
	// kite.AddCommand("create", NewCreate())
	// kite.AddCommand("run", NewRun())
	// kite.AddCommand("stop", NewStop())
	// kite.AddCommand("status", NewStatus())

	// pack := kite.AddModule("pack", "Creates packages")
	// pack.AddCommand("pkg", NewPkg())

	// deploy := kite.AddModule("deploy", "Deploys kite")
	// deploy.AddCommand("remotessh", NewRemoteSSH())

	return &Dispatcher{root: root}
}

func (d *Dispatcher) Run() error {
	command, err := d.findCommand()
	if err != nil {
		return err
	}
	if command != nil {
		return command.Exec()
	}
	return nil
}

func (d *Dispatcher) findCommand() (Command, error) {
	flag.Parse()
	args := flag.Args()
	module, err := d.root.FindModule(args)
	if err != nil {
		return nil, err
	}
	if module != nil {
		return module.Command, nil
	}
	return nil, nil
}
