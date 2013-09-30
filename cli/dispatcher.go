package cli

import (
	"flag"
)

type Dispatcher struct {
	root *Module
}

func NewDispatcher() *Dispatcher {
	root := &Module{Children: make(map[string]*Module, 0), Command: nil}
	root.AddCommand("hello", NewHello())
	root.AddCommand("register", NewKd())
	kite := root.AddModule("kite", "Includes commands related to kites")
	kite.AddCommand("create", NewCreate())
	kite.AddCommand("run", NewRun())

	return &Dispatcher{root: root}
}

func (d *Dispatcher) Run() error {
	command := d.findCommand()
	if command != nil {
		err := command.Exec()
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) findCommand() Command {
	flag.Parse()
	args := flag.Args()
	module := d.root.FindModule(args)
	if module != nil {
		return module.Command
	}
	return nil
}
