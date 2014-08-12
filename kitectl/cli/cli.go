// Package cli is a library to help creating command line tools.
package cli

import (
	"fmt"
	"os"
)

// To add a module, implement this interface. Definition is the command
// definition. Exec is the behaviour that you want to implement as a command
type Command interface {
	Definition() string // usually it's the output for --help
	Exec(args []string) error
}

// Module is the shared structure of commands and sub-commands.
type Module struct {
	children map[string]*Module // Non-nil if sub-command
	command  Command            // Non-nil if command
}

// NewCLI returns a root Module that you can add commands and
// another modules (sub-commands).
func NewCLI() *Module {
	return &Module{
		children: make(map[string]*Module, 0),
	}
}

// AddCommand adds a new command this module.
func (m *Module) AddCommand(name string, command Command) {
	child := &Module{
		command: command,
	}
	m.children[name] = child
}

// AddSubCommand adds a new sub-command this module.
func (m *Module) AddSubCommand(name string) *Module {
	child := &Module{
		children: make(map[string]*Module, 0),
	}
	m.children[name] = child
	return child
}

// Run is the function that is intended to be run from main().
func (m *Module) Run() {
	args := os.Args[1:]

	command, args, err := m.findCommand(args)
	if err != nil {
		exitErr(err)
	}

	err = command.Exec(args)
	if err != nil {
		exitErr(err)
	}

	os.Exit(0) // just to be explicit
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	os.Exit(1)
}

func (m *Module) findCommand(args []string) (Command, []string, error) {
	newArgs := args // this is the subset of args and will be returned with command

	// Iterate over args and update the module pointer "m"
	var arg string
	for _, arg = range args {
		if m == nil || m.children == nil {
			// m is a command
			break
		}

		m = m.children[arg]
		newArgs = newArgs[1:]
	}

	if m == nil {
		return nil, nil, fmt.Errorf("Command not found: %s", arg)
	}

	// m is a command or sub-command we don't care because we are
	// returning Command interface
	return m, newArgs, nil
}

////////////////////////////////////////////////////////////////////////
// Methods below implement Command interface for Module (sub-command) //
////////////////////////////////////////////////////////////////////////

func (m *Module) Definition() string {
	if m.command != nil {
		// m is a command
		return m.command.Definition()
	}

	// m is a sub-command
	return fmt.Sprintf("Run to see sub-commands")
}

func (m *Module) Exec(args []string) error {
	if m.command != nil {
		// m is a command
		return m.command.Exec(args)
	}

	// m is a sub-command
	// Print command list
	fmt.Println("Possible commands:")
	for n, module := range m.children {
		fmt.Printf("  %-10s  ", n)

		if module.command != nil {
			fmt.Println(module.command.Definition())
		} else {
			fmt.Println(module.Definition())
		}
	}

	return nil
}
