package main

import (
	"flag"
	"fmt"

	"github.com/koding/kite"
	"github.com/koding/kite/examples/math"
)

var arg = flag.Int("arg", 4, "An argument to send to the kite server.")

func main() {
	flag.Parse()

	// Create a kite.
	k := kite.New("exp2", "1.0.0")

	// Connect to our math kite.
	mathWorker := k.NewClient(math.Host.URL.String())
	err := mathWorker.Dial()
	if err != nil {
		panic(err)
	}

	// Call square method with the given argument.

	response, err := mathWorker.Tell("square", &math.Request{
		Number: *arg,
		Name:   "math-cli",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println("result:", response.MustFloat64())
}
