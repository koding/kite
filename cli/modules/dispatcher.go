package modules

import (
	"flag"
	"fmt"
	"koding/newkite/cli/core"
	"koding/newkite/cli/modules/hello"
	kitecreate "koding/newkite/cli/modules/kite/create"
	"koding/newkite/cli/modules/register"
	"os"
)

type Dispatcher struct {
	ModuleRoot *core.Module
}

func NewDispatcher() *Dispatcher {
	rootModule := &core.Module{SubModules: make(map[string]*core.Module, 0), Command: nil}
	rootModule.AddCommand("hello", hello.New())
	rootModule.AddCommand("register", register.New())
	kiteModule := rootModule.AddModule("kite", "Includes commands related to kites")
	kiteModule.AddCommand("create", kitecreate.New())
	return &Dispatcher{ModuleRoot: rootModule}
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

func (m *Dispatcher) findCommand() *core.Command {
	flag.Parse()
	treeWalker := m.ModuleRoot
	args := flag.Args()

	for i := 0; i < len(args); i++ {
		if module := treeWalker.SubModules[args[i]]; module != nil {
			if module.Command != nil {
				temp := os.Args
				os.Args = []string{temp[0]}
				os.Args = append(os.Args, temp[i+2:]...)
				return module.Command
			}
			treeWalker = module
		}
	}
	if treeWalker.SubModules != nil {
		fmt.Fprintf(os.Stderr, "Possible commands: \n")
		for commandName, module := range treeWalker.SubModules {
			fmt.Fprintf(os.Stderr, "%s - ", commandName)
			if module.Command != nil {
				fmt.Fprintf(os.Stderr, "%s", module.Command.Help())
			} else {
				fmt.Fprintf(os.Stderr, "%s", module.Definition)
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	return nil
}
