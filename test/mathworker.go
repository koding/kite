package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"strconv"
)

type Math struct{}

func (Math) Square(r *protocol.KiteRequest, result *string) error {
	*result = strconv.Itoa(int(r.Args.(float64)) * int(r.Args.(float64)))
	fmt.Printf("[%s] call, sending result '%s' back\n", r.Origin, *result)
	return nil
}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	o := &protocol.Options{
		Username: "fatih",
		Kitename: "mathworker",
		Version:  "1",
		Port:     *port,
	}

	k := kite.New(o, new(Math))
	k.Start()
}
