// Package cli is a library to help creating command line tools.
package cli

import (
	"flag"
	"fmt"
	"os"
)

// To add a module, implement this interface. Definition is the command
// definition. Exec is the behaviour that you want to implement as a command
type Command interface {
	Definition() string // usually it's the output for --help
	Exec() error
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
	flag.Parse()
	args := flag.Args()

	command, err := m.findCommand(args)
	if err != nil {
		exitErr(err)
	}

	err = command.Exec()
	if err != nil {
		exitErr(err)
	}

	os.Exit(0)
}

func (m *Module) findCommand(args []string) (Command, error) {
	// Iterate over args and update the module pointer "m"
	for _, arg := range args {
		// Treat m as a module (sub-command)
		sub := m.children[arg]
		if sub == nil {
			return nil, fmt.Errorf("Command not found")
		}

		// sub is another module here
		if sub.command == nil {
			m = m.children[arg]
			continue
		}

		args = args[1:]

		// Returning command module
		return sub, nil
	}

	// Returning sub-command module
	return m, nil
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	os.Exit(1)
}

////////////////////////////////////////////////////////////////////////
// Methods below implement Command interface for Module (sub-command) //
////////////////////////////////////////////////////////////////////////

func (m *Module) Definition() string {
	if m.command != nil {
		return m.command.Definition()
	}

	return fmt.Sprintf("Run to see sub-commands")
}

func (m *Module) Exec() error {
	if m.command != nil {
		return m.command.Exec()
	}

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
