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
	hello3 := NewHello("hello3")

	root := NewCLI()
	root.AddCommand("hello", hello)

	s := root.AddSubCommand("sub")
	s.AddCommand("hello2", hello2)

	s2 := s.AddSubCommand("sub2")
	s2.AddCommand("hello3", hello3)

	type TestCase struct {
		fullArgs    string
		expectError bool
		definition  string
		args        string
	}

	cases := []TestCase{
		{"notExist", true, "", ""},
		{"hello", false, hello.Definition(), ""}, // test first level command
		{"hello asdf", false, hello.Definition(), "asdf"},
		{"sub", false, "Run to see sub-commands", ""},
		{"sub notExist", true, "", ""},
		{"sub hello2", false, hello2.Definition(), ""}, // second level
		{"sub hello2 asdf", false, hello2.Definition(), "asdf"},
		{"sub sub2 hello3 asdf", false, hello3.Definition(), "asdf"}, // third level
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
