package cli

import (
	"flag"
	"fmt"
	"os"
)

type Dispatcher struct {
	root *Module
}

func NewDispatcher() *Dispatcher {
	root := &Module{SubModules: make(map[string]*Module, 0), Command: nil}
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
	if len(args) == 0 {
		printPossibleCommands(m.root)
		return nil
	}
	moduleWalker := m.root
	for i := 0; i < len(args); i, moduleWalker = i+1, moduleWalker.SubModules[args[i]] {
		module := moduleWalker.SubModules[args[i]]
		if module == nil {
			fmt.Printf("Command %s not found\n\n", args[i])
			break
		}
		if module.Command == nil {
			continue
		}
		temp := os.Args
		os.Args = []string{temp[0]}
		os.Args = append(os.Args, temp[i+2:]...)
		return module.Command
	}
	if moduleWalker.SubModules != nil {
		printPossibleCommands(moduleWalker)
	}
	return nil
}

func printPossibleCommands(module *Module) {
	fmt.Println("Possible commands: ")
	for n, m := range module.SubModules {
		fmt.Printf("%s - ", n)
		if m.Command != nil {
			fmt.Printf("%s\n", m.Command.Help())
		} else {
			fmt.Printf("%s\n", m.Definition)
		}
	}
}
