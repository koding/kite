package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"math/rand"
	"time"
)

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()

	options := &kite.Options{
		Kitename:    "application",
		Version:     "1",
		Port:        *port,
		Region:      "localhost",
		Environment: "development",
		Username:    "devrim",
	}

	k := kite.New(options)
	go k.Run()

	// this is needed that the goroutine k.Start() is been settled. We will
	// probably change the behaviour of k.Start() from blocking to nonblocking
	// and remove the sleep, however this is a design decision that needs to be
	// rethought.
	time.Sleep(1 * time.Second)

	query := protocol.KontrolQuery{
		Username:    "devrim",
		Environment: "development",
		Name:        "mathworker",
	}

	// To demonstrate we can receive notifications matcing to our query.
	onEvent := func(e *protocol.KiteEvent) {
		fmt.Printf("--- kite event: %#v\n", e)
	}

	kites, err := k.Kontrol.GetKites(query, onEvent)
	if err != nil {
		fmt.Println(err)
		return
	}

	if len(kites) == 0 {
		fmt.Println("No mathworker available")
		return
	}

	mathWorker := kites[0]
	err = mathWorker.Dial()
	if err != nil {
		fmt.Println("Cannot connect to remote mathworker")
		return
	}

	squareOf := func(i int) {
		response, err := mathWorker.Call("square", i)
		if err != nil {
			fmt.Println(err)
			return
		}

		var result int
		err = response.Unmarshal(&result)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("input: %d  rpc result: %d\n", i, result)
	}

	for {
		squareOf(rand.Intn(10))
		time.Sleep(time.Second)
	}
}
