package main

import (
	"flag"
	"fmt"

	"github.com/koding/kite"
	"github.com/koding/kite/examples/math"
)

func main() {
	flag.Parse()

	// Create a kite.
	k := kite.New("math", "1.0.0")

	// Add pre handler method.
	k.PreHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("\nThis pre handler is executed before the method is executed")
		resp := "hello from pre handler!"

		// let us return an hello to base square method!
		r.Context.Set("response", resp)
		return resp, nil
	})

	// Add post handler method.
	k.PostHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("This post handler is executed after the method is executed")

		// Pass the response from the previous square method back to the
		// client, this is imporant if you use post handler.
		return r.Context.Get("response")
	})

	// Add our handler method, authentication is disabled for this example.
	k.HandleFunc("square", Square).DisableAuthentication().PreHandleFunc(func(r *kite.Request) (interface{}, error) {
		fmt.Println("This pre handler is only valid for this individual method")
		return nil, nil
	})

	// Attach to a server and run it.
	k.Config.IP = math.Host.IP()
	k.Config.Port = math.Host.Port()
	k.Run()
}

func Square(r *kite.Request) (interface{}, error) {
	// Unmarshal method arguments.
	var params math.Request
	if err := r.Args.One().Unmarshal(&params); err != nil {
		return nil, err
	}

	result := params.Number * params.Number

	fmt.Printf("Call received from '%s', sending result '%.0d' back\n", params.Name, result)

	// Print a log on remote Kite.
	// This message will be printed on client's console.
	r.Client.Go("kite.log", fmt.Sprintf("Message from %s: \"You have requested square of %.0d\"", r.LocalKite.Kite().Name, params.Number))

	// You can return anything as result, as long as it is JSON marshalable.
	return result, nil
}
