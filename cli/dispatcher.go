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
	root.AddCommand("register", NewRegister())
	kite := root.AddModule("kite", "Includes commands related to kites")
	kite.AddCommand("create", NewCreate())
	kite.AddCommand("run", NewRun())

	return &Dispatcher{root: root}
}

func (m *Dispatcher) Run() error {
	command := m.findCommand()
	if command != nil {
		err := command.Exec()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Dispatcher) findCommand() Command {
	flag.Parse()
	args := flag.Args()
	module := m.root.FindModule(args)
	if module != nil {
		return module.Command
	}
	return nil
}
