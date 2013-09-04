package main

import (
	"kite/kite"
	"kite/protocol"
)

type Foo struct{}

func (Foo) Func(r *protocol.KiteRequest, result *string) error {
	return nil
}

func main() {
	k := kite.New(&protocol.Options{
		Username:     "fatih",
		Kitename:     "foo",
		Version:      "1",
		Dependencies: "bar",
	}, new(Foo))

	k.Start()
}
