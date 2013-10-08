package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"strings"
)

var collector = make(chan msg)

type msg struct {
	data string
	from string
	to   []string
}

type Chat struct{}

func (Chat) Inbox(r *protocol.KiteRequest, result *string) error {
	// TODO: check for kindness via reflect before type assertion, otherwise
	// you'll get a panic because the type assertion occurs at runtime
	args := r.Args.(map[string]interface{})
	d := args["msg"].(string)
	t := strings.Split(args["to"].(string), ",")
	collector <- msg{data: d, from: r.Username, to: t}
	return nil
}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	k := kite.New(&protocol.Options{
		Username:     "fatih",
		Kitename:     "chatworker",
		Version:      "2",
		Port:         *port,
		Dependencies: "",
	}, new(Chat))

	go k.Start()

	for msg := range collector {
		fmt.Printf("send msg: '%s' from '%s' to '%v'\n", msg.data, msg.from, msg.to)
		for _, t := range msg.to {
			k.SendMsg(msg.data, strings.ToLower(t))
		}
	}
}
