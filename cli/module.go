package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// To add a module, implement this interface
// Definition is the command definition
// Exec is the behaviour that you want to implement as a command
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
	current := m
	var errStr bytes.Buffer
	for i, arg := range args {
		sub := current.Children[arg]
		if sub == nil {
			errStr.WriteString(fmt.Sprintf("Command %s not found\n\n", arg))
			break
		}
		if sub.Command == nil {
			current = current.Children[arg]
			continue
		}
		// command behaves like a subprocess, it will parse arguments again
		// so we re discarding parsed arguments
		temp := os.Args
		os.Args = []string{temp[0]}
		os.Args = append(os.Args, temp[i+2:]...)
		return sub, nil
	}
	errStr.WriteString(current.printPossibleCommands())
	return nil, errors.New(errStr.String())
}

func (m *Module) printPossibleCommands() string {
	var buffer bytes.Buffer
	buffer.WriteString("Possible commands: \n")
	for n, module := range m.Children {
		buffer.WriteString(fmt.Sprintf("  %-10s  ", n))
		var definition string
		if module.Command != nil {
			definition = module.Command.Definition()
		} else {
			definition = module.Definition
		}
		buffer.WriteString(fmt.Sprintf("%s\n", definition))
	}
	return buffer.String()
}
