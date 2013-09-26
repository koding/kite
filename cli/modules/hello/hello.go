package hello

import (
	"flag"
	"fmt"
	"koding/newkite/cli/core"
)

func New() *core.Command {
	helper := func() string {
		return "Hellos a world"
	}
	executer := func() error {
		// flags should be defined locally, there are lots of flag parsing
		// at other tools
		var port = flag.Int("port", 4010, "port number")
		flag.Parse()
		fmt.Printf("port:%d\n", *port)
		fmt.Println("Wawing hand\n")
		return nil
	}

	return &core.Command{Help: helper, Exec: executer}
}
