package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
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
		Name:        "datastore",
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
		fmt.Println("No datastore kite is available")
		return
	}

	datastore := kites[0]
	fmt.Println(datastore)
	err = datastore.Dial()
	if err != nil {
		fmt.Println("Cannot connect to remote datastore kite")
		return
	}

	set := func(k string, v string) {
		// we cant simply call, right ??
		// response, err := datastore.Call("set", k, v )
		response, err := datastore.Call("set", []string{k, v} )
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

		fmt.Printf("input: %d  rpc result: %d\n", k, result)
	}


	get := func(k string) (error, string) {
		// we cant simply call, right ??
		// response, err := datastore.Call("set", k, v )
		response, err := datastore.Call("get", k )
		if err != nil {
			fmt.Println(err)
			return err, ""
		}

		var result string
		err = response.Unmarshal(&result)
		if err != nil {
			fmt.Println(err)
			return err, ""
		}

		fmt.Printf("input: %d  rpc result: %d\n", k, result)
		return err, result
	}

	for {
		set("foo", "bar")
		_, v := get("foo")
		fmt.Println("get foo >>>", v == "bar")
		time.Sleep(time.Second)
	}
}
