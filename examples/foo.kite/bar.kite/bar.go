package main

import (
	"kite/kite"
	"kite/protocol"
)

type Bar struct{}

func (Bar) Func(r *protocol.KiteRequest, result *string) error {
	return nil
}

func main() {
	k := kite.New(&protocol.Options{
		Username: "fatih",
		Kitename: "bar",
		Version:  "1",
		Port:     *port,
	}, new(Bar))

	k.Start()
}
