package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
)

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()

	options := &protocol.Options{
		Kitename:    "mathworker",
		Version:     "1",
		Port:        *port,
		Region:      "localhost",
		Environment: "development",
	}

	k := kite.New(options)

	k.HandleFunc("square", Square)

	k.Run()
}

func Square(r *kite.Request) (interface{}, error) {
	a, err := r.Args.Float64()
	if err != nil {
		return nil, err
	}

	result := a * a

	fmt.Printf("Kite call, sending result '%s' back\n", result)

	// Print a log on remote Kite.
	r.RemoteKite.Go("log", fmt.Sprintf("You have requested square of: %f", a))

	return result, nil
}
