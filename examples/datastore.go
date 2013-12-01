package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
)

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()

	options := &kite.Options{
		Kitename:    "datastore",
		Version:     "1",
		Port:        *port,
		Region:      "localhost",
		Environment: "development",
		PublicIP:    "127.0.0.1",
	}

	k := kite.New(options)

	k.HandleFunc("set", Set)
	k.HandleFunc("get", Get)

	k.Run()
}


func Get(r *kite.Request) (interface{}, error) {
	key, err := r.Args.String()
	if err != nil {
		return nil, err
	}
	fmt.Println("get called with - ", key)
	result := "some string"
	return result, nil
}

func Set(r *kite.Request) (interface{}, error) {
	kv, err := r.Args.Array()
	if err != nil {
		return nil, err
	}

	fmt.Println("set called with - ", kv)

	result := 1
	return result, nil
}
