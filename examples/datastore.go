package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/db/mongodb/modelhelper"
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


func Set(r *kite.Request) (interface{}, error) {
	kv, err := r.Args.Array()
	if err != nil {
		return nil, err
	}

	keyValue := modelhelper.NewKeyValue(r.Username, r.RemoteKite.ID, kv[0].(string), kv[1].(string))
	err = modelhelper.UpsertKeyValue(keyValue)
	fmt.Println("set called with - ", kv, keyValue)
	result := true
	if err != nil {
		result = false
	}
	return result, err
}

func Get(r *kite.Request) (interface{}, error) {
	key, err := r.Args.String()
	if err != nil {
		return nil, err
	}

	fmt.Println("requesting user :", r.Username, " kite:", r.RemoteKite)
	fmt.Println("get called with - ", key)

	kv, err := modelhelper.GetKeyValue(r.Username, r.RemoteKite.ID, key)
	if err != nil{
		return err, nil
	}

	return kv.Value, nil
}
