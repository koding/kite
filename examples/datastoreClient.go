package main

import (
	"flag"
	"fmt"
	"koding/kite"
	"koding/kite/protocol"
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
	k.Start()

	query := protocol.KontrolQuery{
		Username:    "devrim",
		Environment: "development",
		Name:        "datastore",
	}

	// To demonstrate we can receive notifications matcing to our query.
	onEvent := func(e *kite.Event) {
		fmt.Printf("--- kite event: %#v\n", e)
	}

	kites, err := k.Kontrol.GetKites(query, onEvent)
	if err != nil {
		fmt.Println(err)
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
		response, err := datastore.Call("set", []string{k, v})
		if err != nil {
			fmt.Println(err)
			return
		}

		var result bool
		err = response.Unmarshal(&result)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("input: %d  rpc result: %d\n", k, result)
	}

	get := func(k string) (error, string) {
		response, err := datastore.Call("get", k)
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
