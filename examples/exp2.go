package main

import (
	"flag"
	"fmt"
	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
	"math/rand"
	"time"
)

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()

	options := &kite.Options{
		Kitename:    "application",
		Version:     "0.0.1",
		Port:        *port,
		Region:      "localhost",
		Environment: "development",
	}

	k := kite.New(options)
	k.Start()

	query := protocol.KontrolQuery{
		Username:    k.Username,
		Environment: "development",
		Name:        "mathworker",
	}

	// To demonstrate we can receive notifications matcing to our query.
	onEvent := func(e *kite.Event, err error) {
		fmt.Printf("--- kite event: %#v\n", e)
		fmt.Printf("--- e.Kite.URL.String(): %+v\n", e.Kite.URL.String())
	}

	go func() {
		_, err := k.Kontrol.WatchKites(query, onEvent)
		if err != nil {
			fmt.Println(err)
		}
	}()

	// .. or just get the current kites and dial for one
	kites, err := k.Kontrol.GetKites(query)
	if err != nil {
		fmt.Println(err)
		return
	}

	mathWorker := kites[0]
	err = mathWorker.Dial()
	if err != nil {
		fmt.Println("Cannot connect to remote mathworker")
		return
	}

	squareOf := func(i int) {
		response, err := mathWorker.Tell("square", i)
		if err != nil {
			fmt.Println(err)
			return
		}

		result := response.MustFloat64()
		fmt.Printf("input: %d  rpc result: %f\n", i, result)
	}

	for {
		squareOf(rand.Intn(10))
		time.Sleep(time.Second)
	}
}
