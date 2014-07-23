package main

import (
	"fmt"

	"github.com/koding/kite"
)

func main() {
	// Create a kite
	k := kite.New("math", "1.0.0")

	// Add pre handler method
	k.PreHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("\nThis pre handler is executed before the method is executed")

		// let us return an hello to base square method!
		return "hello from pre handler!", nil
	})

	// Add post handler method
	k.PostHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("This post handler is executed after the method is executed")

		// pass the response from the previous square method back to the
		// client, this is imporant if you use post handler
		return r.Response, nil
	})

	// Add our handler method, authentication is disabled for this example
	k.HandleFunc("square", Square).DisableAuthentication().PreHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("This pre handler is only valid for this individual method")
		return nil, nil
	})

	// Attach to a server and run it
	k.Config.Port = 3636
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
