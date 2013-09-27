package hello

import (
	"flag"
	"fmt"
)

type Hello struct{}

func NewHello() *Hello {
	return &Hello{}
}

func (h Hello) Help() string {
	return "Hellos a world"
}

func (h Hello) Exec() error {
	// flags should be defined locally, there are lots of flag parsing
	// at other tools
	var port = flag.Int("port", 4010, "port number")
	flag.Parse()
	fmt.Printf("port:%d\n", *port)
	fmt.Println("Wawing hand\n")
	return nil
}
