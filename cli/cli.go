package main

import (
	"fmt"
	"koding/newkite/cli/dispatcher"
	"os"
)

func main() {
	d := dispatcher.NewDispatcher()
	err := d.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	}
}
