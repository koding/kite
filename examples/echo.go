// Echo kite is an example to kite that just echoes the argument back.
// Useful for debugging or testing stuff.
package main

import "kite"

func main() {
	options := &kite.Options{
		Kitename:    "echo",
		Version:     "0.0.1",
		Environment: "development",
		Region:      "localhost",
	}

	k := kite.New(options)

	k.HandleFunc("echo", func(r *kite.Request) (interface{}, error) {
		return r.Args.One().MustString(), nil
	})

	k.Run()
}
