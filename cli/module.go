package cli

import (
	"errors"
	"fmt"
	"os"
)

type Command interface {
	Definition() string
	Exec() error
}

type Module struct {
	Children   map[string]*Module
	Command    Command
	Definition string
}

func NewModule(name string, definition string) *Module {
	return &Module{Children: make(map[string]*Module, 0), Definition: definition}
}

func NewCommandModule(command Command) *Module {
	return &Module{Command: command}
}

func (m *Module) AddCommand(name string, command Command) *Module {
	child := NewCommandModule(command)
	m.Children[name] = child
	return child
}

func (m *Module) AddModule(name string, definition string) *Module {
	child := NewModule(name, definition)
	m.Children[name] = child
	return child
}

func (m *Module) FindModule(args []string) (*Module, error) {
	var err error = nil
	moduleWalker := m
	for i := 0; i < len(args); i, moduleWalker = i+1, moduleWalker.Children[args[i]] {
		module := moduleWalker.Children[args[i]]
		if module == nil {
			err = errors.New(fmt.Sprintf("Command %s not found\n\n", args[i]))
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
		return module, err
	}
	printPossibleCommands(moduleWalker)
	return nil, err
}

func printPossibleCommands(module *Module) {
	fmt.Println("Possible commands: ")
	for n, m := range module.Children {
		fmt.Printf("%s - ", n)
		if m.Command != nil {
			fmt.Printf("%s\n", m.Command.Definition())
		} else {
			fmt.Printf("%s\n", m.Definition)
		}
	}
}
