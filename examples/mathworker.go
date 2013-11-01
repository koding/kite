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

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	options := &protocol.Options{
		Kitename: "mathworker",
		Version:  "1",
		Port:     *port,
	}

	methods := map[string]string{
		"math.square": "Square",
	}

	k := kite.New(options)
	k.AddMethods(new(Math), methods)
	k.Start()
}

func (Math) Square(r *protocol.KiteRequest, result *string) error {
	a, ok := r.Args.(float64)
	if !ok {
		return errors.New("Send float64")
	}
	b := int(a)
	*result = strconv.Itoa(b * b)

	fmt.Printf("Kite call, sending result '%s' back\n", *result)
	return nil
}
