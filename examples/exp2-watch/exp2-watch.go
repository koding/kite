// TODO: Watcher was disabled by e8ad10d.

// +build ignore

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

	onEvent := func(e *kite.Event, err *kite.Error) {
		fmt.Printf("e %+v\n", e)
		fmt.Printf("err %+v\n", err)
	}

	_, err := k.WatchKites(protocol.KontrolQuery{
		Username:    k.Config.Username,
		Environment: k.Config.Environment,
		Name:        "math",
		// ID: "48bb002b-79f6-4a4e-6bba-a40567a08b6c",
	}, onEvent)
	if err != nil {
		log.Fatalln(err)
	}

	// This is a bad example, it's just for testing the watch functionality :)
	fmt.Println("listening to events")

	select {}
}
