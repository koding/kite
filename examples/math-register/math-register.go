package main

import (
	"fmt"
	"log"
	"net/url"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
)

func main() {
	// Create a kite
	k := kite.New("math", "1.0.0")

	// Add our handler method
	k.HandleFunc("square", Square)

	// Get config from kite.Key directly, usually it's under ~/.kite/kite.key
	config := config.MustGet()
	k.Config = config
	k.Config.Port = 6667
	k.Id = config.Id

	// Register to kite with this url
	kiteURL := &url.URL{Scheme: "http", Host: "localhost:6667", Path: "/kite"}

	// Register us ...
	_, err := k.Register(kiteURL)
	if err != nil {
		log.Fatal(err)
	}

	// And finally attach to a server and run it
	k.Run()
}

func Square(r *kite.Request) (interface{}, error) {
	// Unmarshal method arguments
	a := r.Args.One().MustFloat64()

	result := a * a

	fmt.Printf("Call received, sending result %.0f back\n", result)

	// Print a log on remote Kite.
	// This message will be printed on client's console.
	r.Client.Go("kite.log", fmt.Sprintf("Message from %s: \"You have requested square of %.0f\"", r.LocalKite.Kite().Name, a))

	// You can return anything as result, as long as it is JSON marshalable.
	return result, nil
}
