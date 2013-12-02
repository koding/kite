// this provides a simple datastore for kites just with get/set methods.
// mongodb has 24k number of collection limit in a single database
// http://stackoverflow.com/questions/9858393/limits-of-number-of-collections-in-databases
// thats why we have a single collection and use single index
// though instead of using a single collection  we can use different strategies, like
// multiple database, single collections
// multiple database, multiple collections
// etc... to make it a bit more performant.
// though mongodb has an auto sharding setup, http://docs.mongodb.org/manual/sharding/
// which should be considered first. or use another datastore like elasticsearch, cassandra etc.
// to handle the sharding on database level.
// thats why we only have one strategy only for now, to get the ball rolling.

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
