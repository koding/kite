package cli

import (
	"fmt"
	"os"
)

type Command interface {
	Help() string
	Exec() error
}

type Module struct {
	Children   map[string]*Module
	Command    Command
	Definition string
}

func NewModule() *Module {
	return &Module{
		Children: make(map[string]*Module, 0),
	}
}

func (m *Module) AddCommand(name string, command Command) *Module {
	child := &Module{Command: command}
	m.Children[name] = child
	return child
}

func (m *Module) AddModule(name string, definition string) *Module {
	child := &Module{Children: make(map[string]*Module, 0), Definition: definition}
	m.Children[name] = child
	return child
}

func (m *Module) FindModule(args []string) *Module {
	moduleWalker := m

	for i := 0; i < len(args); i, moduleWalker = i+1, moduleWalker.Children[args[i]] {
		module := moduleWalker.Children[args[i]]
		if module == nil {
			fmt.Printf("Command %s not found\n\n", args[i])
			break
		}
		if module.Command == nil {
			continue
		}
		// command behaves like a subprocess, it will parse arguments again
		// so we re discarding parsed arguments
		temp := os.Args
		os.Args = []string{temp[0]}
		os.Args = append(os.Args, temp[i+2:]...)
		return module
	}
	printPossibleCommands(moduleWalker)
	return nil
}

func printPossibleCommands(module *Module) {
	fmt.Println("Possible commands: ")
	for n, m := range module.Children {
		fmt.Printf("%s - ", n)
		if m.Command != nil {
			fmt.Printf("%s\n", m.Command.Help())
		} else {
			fmt.Printf("%s\n", m.Definition)
		}
	}
}
