package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/protocol"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// Create a kite
	k := kite.New("exp2", "1.0.0")
	k.Config = config.MustGet()

	kites, err := k.GetKites(&protocol.KontrolQuery{
		Username:    k.Config.Username,
		Environment: k.Config.Environment,
		Name:        "math",
	})
	if err != nil {
		log.Fatalln(err)
	}

	// Connect to remote kite
	defer kite.Close(kites[1:])

	mathWorker := kites[0]
	connected, err := mathWorker.DialForever()
	if err != nil {
		k.Log.Fatal(err.Error())
	}

	// Wait until connected
	<-connected

	// Call square method every second
	for range time.Tick(time.Second) {
		i := rand.Intn(10)

		// Call a method of mathworker kite
		response, err := mathWorker.TellWithTimeout("square", 4*time.Second, i)
		if err != nil {
			k.Log.Error(err.Error())
			continue
		}

		// Print the result
		result := response.MustFloat64()
		fmt.Printf("input: %d  result: %.0f\n", i, result)
	}
}
