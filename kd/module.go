package kd

import (
	"errors"
	"fmt"
	"os"
)

// To add a module, implement this interface. Definition is the command
// definition. Exec is the behaviour that you want to implement as a command
type Command interface {
	Definition() string // usually it's the output for --help
	Exec() error
}

type Module struct {
	Children   map[string]*Module
	Command    Command
	Definition string
}

func NewModule(command Command, definition string) *Module {
	return &Module{
		Children:   make(map[string]*Module, 0),
		Definition: definition,
		Command:    command,
	}
}

func (m *Module) AddCommand(name string, command Command) *Module {
	child := NewModule(command, "")
	m.Children[name] = child
	return child
}

func (m *Module) AddModule(name string, definition string) *Module {
	child := NewModule(nil, definition)
	m.Children[name] = child
	return child
}

func (m *Module) FindModule(args []string) (*Module, error) {
	var err = errors.New("")
	for i, arg := range args {
		sub := m.Children[arg]
		if sub == nil {
			err = fmt.Errorf("Command %s not found\n\n", arg)
			break
		}

		if sub.Command == nil {
			m = m.Children[arg]
			continue
		}

		// command behaves like a subprocess, it will parse arguments again
		// so we re discarding parsed arguments
		temp := os.Args
		os.Args = []string{temp[0]}
		os.Args = append(os.Args, temp[i+2:]...)
		return sub, nil
	}
	err = fmt.Errorf("%s%s", err, m.printPossibleCommands())
	return nil, err
}

func (m *Module) printPossibleCommands() string {
	prompt := "Possible commands: \n"
	for n, module := range m.Children {
		prompt += fmt.Sprintf("  %-10s  ", n)
		definition := module.Definition
		if module.Command != nil {
			definition = module.Command.Definition()
		}
		prompt += fmt.Sprintf("%s\n", definition)
	}
	return prompt
}
