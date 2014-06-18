package main

import (
	"flag"
	"fmt"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testutil"
	"os"
)

var (
	flagEnableOutput = flag.Bool("stdout", false, "Output raw token to std out.")
)

func main() {
	flag.Parse()

	if *flagEnableOutput {
		fmt.Printf("%v", testutil.NewKiteKey().Raw)
		os.Exit(0)
	}

	kitekey.Write(testutil.NewKiteKey().Raw)
}
