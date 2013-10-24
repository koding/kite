package main

import (
	"fmt"
	"koding/newkite/kd"
	"os"
)

func main() {
	d := kd.NewDispatcher()
	err := d.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
