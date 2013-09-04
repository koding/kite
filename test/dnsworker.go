package main

import (
	"fmt"
	"koding/newkite/kite"
	"net"
)

type Dns struct{}

func (Dns) Resolve(args string, result *[]net.IP) error {
	fmt.Println("got a call to Resolve()")
	ips, err := net.LookupIP(args)
	if err != nil {
		fmt.Println(err)
	}
	*result = ips
	return nil
}

func (Dns) SplitHostPort(args string, result *[]string) error {
	fmt.Println("got a call to SplitHostPort()")
	host, port, _ := net.SplitHostPort(args)
	res := make([]string, 0)
	res = append(res, host)
	res = append(res, port)
	*result = res
	return nil
}

func main() {
	k := kite.New("dnsworker", new(Dns))
	k.Start()
}
