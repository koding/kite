package dispatcher

import (
	"flag"
	"fmt"
	"koding/newkite/cli/module"
	"koding/newkite/cli/modules/hello"
	"koding/newkite/cli/modules/kite"
	"koding/newkite/cli/modules/register"
	"os"
)

type Dispatcher struct {
	ModuleRoot *module.Module
}

func NewDispatcher() *Dispatcher {
	rootModule := &module.Module{SubModules: make(map[string]*module.Module, 0), Command: nil}
	rootModule.AddCommand("hello", hello.NewHello())
	rootModule.AddCommand("register", register.NewRegister())
	kiteModule := rootModule.AddModule("kite", "Includes commands related to kites")
	kiteModule.AddCommand("create", kite.NewCreate())
	kiteModule.AddCommand("run", kite.NewRun())

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

func (m *Dispatcher) findCommand() module.Command {
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
