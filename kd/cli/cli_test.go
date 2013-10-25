package cli

import (
	"fmt"
	"strings"
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

	type TestCase struct {
		fullArgs    string
		expectError bool
		definition  string
		args        string
	}

	cases := []TestCase{
		TestCase{"notExist", true, "", ""},
		TestCase{"hello", false, hello.Definition(), ""},
		TestCase{"hello asdf", false, hello.Definition(), "asdf"},
		TestCase{"sub", false, "Run to see sub-commands", ""},
		TestCase{"sub notExist", true, "", ""},
		TestCase{"sub hello2", false, hello2.Definition(), ""},
		TestCase{"sub hello2 asdf", false, hello2.Definition(), "asdf"},
	}

	for _, c := range cases {
		fmt.Println("======================================================================")
		fmt.Println("Testing", c)

		cmd, args, err := root.findCommand(strings.Split(c.fullArgs, " "))
		if err != nil {
			if !c.expectError {
				t.Error("Error expected but not found")
			}

			continue
		}

		if cmd.Definition() != c.definition {
			t.Error("Definition is not correct:", cmd.Definition())
		}

		if strings.Join(args, " ") != c.args {
			t.Error("Invalid args:", args, "!=", c.args)
		}
	}
}
