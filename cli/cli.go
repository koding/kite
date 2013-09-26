package main

import (
	"fmt"
	"koding/newkite/cli/modules"
	"os"
)

func main() {
	dispatcher := modules.NewDispatcher()
	err := dispatcher.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	}
}
