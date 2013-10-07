package main

import (
	"fmt"
	"koding/newkite/cli"
	"os"
)

func main() {
	d := cli.NewDispatcher()
	err := d.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	}
}
