package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/koding/kite"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// Create a kite
	k := kite.New("exp2", "1.0.0")

	// Create mathworker client
	mathWorker := k.NewClientString("ws://localhost:3636")

	// Connect to remote kite
	connected, err := mathWorker.DialForever()
	if err != nil {
		k.Log.Fatal(err.Error())
	}

	// Wait until connected
	<-connected

	// Call square method every second
	for _ = range time.Tick(time.Second) {
		i := rand.Intn(10)

		// Call a method of mathworker kite
		response, err := mathWorker.Tell("square", i)
		if err != nil {
			k.Log.Error(err.Error())
			continue
		}

		// Print the result
		result := response.MustFloat64()
		fmt.Printf("input: %d  result: %.0f\n", i, result)
	}
}
