package cli

import (
	"fmt"
	"testing"
)

type Hello struct{ name string }

func NewHello(name string) *Hello { return &Hello{name} }

func (h *Hello) Definition() string {
	return "Say hello to " + h.name
}

func (h *Hello) Exec(args []string) error {
	fmt.Println("hello")
	return nil
}

func TestFindCommand(t *testing.T) {
	hello := NewHello("hello")
	hello2 := NewHello("hello2")

	root := NewCLI()
	root.AddCommand("hello", hello)

	s := root.AddSubCommand("sub")
	s.AddCommand("hello2", hello2)

	println("==========")
	_, _, err := root.findCommand([]string{"notExist"})
	if err == nil {
		t.Error("error")
	}

	println("==========")
	cmd, _, err := root.findCommand([]string{"hello"})
	if err != nil {
		t.Error(err.Error())
	}
	if cmd.Definition() != hello.Definition() {
		t.Error("error")
	}

	println("==========")
	_, _, err = s.findCommand([]string{"notExist"})
	if err == nil {
		t.Error("error")
	}

	println("==========")
	cmd, _, err = root.findCommand([]string{"sub", "hello2"})
	if err != nil {
		t.Error(err.Error())
	}
	if cmd.Definition() != hello2.Definition() {
		t.Error("error")
	}

	println("==========")
	_, _, err = root.findCommand([]string{"sub", "notExist"})
	if err == nil {
		t.Error("error")
	}
}
