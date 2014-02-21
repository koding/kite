package main

import (
	"flag"
	"fmt"
	"github.com/koding/kite"
)

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()

	options := &kite.Options{
		Kitename:    "mathworker",
		Version:     "0.0.1",
		Port:        *port,
		Region:      "localhost",
		Environment: "development",
	}

	k := kite.New(options)

	k.HandleFunc("square", Square)

	k.Run()
}

func Square(r *kite.Request) (interface{}, error) {
	a := r.Args.One().MustFloat64()

	result := a * a

	fmt.Printf("Kite call, sending result %.0f back\n", result)

	// Print a log on remote Kite.
	r.RemoteKite.Go("log", fmt.Sprintf("You have requested square of: %f", a))

	return result, nil
}
