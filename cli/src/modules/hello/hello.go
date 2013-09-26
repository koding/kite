package hello

import (
	"core"
	"flag"
	"fmt"
)

func New() *core.Command {
	helper := func() string {
		return "Hellos a world"
	}
	executer := func() error {
		var port = flag.Int("port", 4010, "port number")
		flag.Parse()
		fmt.Printf("port:%d\n", *port)
		fmt.Println("Wawing hand\n")
		return nil
	}

	return &core.Command{Help: helper, Exec: executer}
}
