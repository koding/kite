package main

import (
	"errors"
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"strconv"
)

type Math struct{}

func (Math) Square(r *protocol.KiteRequest, result *string) error {
	a, ok := r.Args.(float64)
	if !ok {
		return errors.New("Send float64")
	}
	b := int(a)
	*result = strconv.Itoa(b * b)

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

	methods := map[string]interface{}{
		"math.square": Math.Square,
	}
	k := kite.New(o, new(Math), methods)
	k.Start()
}
