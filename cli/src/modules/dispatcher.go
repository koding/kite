package modules

import (
	"core"
	"flag"
	"fmt"
	"modules/hello"
	kitecreate "modules/kite/create"
	"modules/register"
	"os"
)

type Dispatcher struct {
	ModuleRoot *core.ModuleNode
}

func NewDispatcher() *Dispatcher {
	rootModule := &core.ModuleNode{SubModules: make(map[string]*core.ModuleNode, 0), Command: nil}
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
		if moduleNode := treeWalker.SubModules[args[i]]; moduleNode != nil {
			if moduleNode.Command != nil {
				os.Args = os.Args[i+2:]
				return moduleNode.Command
			}
			treeWalker = moduleNode
		}
	}
	if treeWalker.SubModules != nil {
		fmt.Fprintf(os.Stderr, "Possible commands: \n")
		for commandName, moduleNode := range treeWalker.SubModules {
			fmt.Fprintf(os.Stderr, "%s - ", commandName)
			if moduleNode.Command != nil {
				fmt.Fprintf(os.Stderr, "%s", moduleNode.Command.Help())
			} else {
				fmt.Fprintf(os.Stderr, "%s", moduleNode.ModuleDefinition)
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	return nil
}
