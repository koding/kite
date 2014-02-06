package main

import (
	"flag"
	"kite"
	"kite/kontrol"
	"kite/testkeys"
)

func main() {
	options := &kite.Options{
		Kitename: "kontrol",
		Version:  "0.0.1",
		Path:     "/kontrol",
	}

	flag.StringVar(&options.Environment, "environment", "development", "")
	flag.StringVar(&options.Region, "region", "localhost", "")
	flag.StringVar(&options.PublicIP, "ip", "0.0.0.0", "")
	flag.StringVar(&options.Port, "port", "4000", "")

	flag.Parse()

	k := kontrol.New(options, nil, testkeys.Public, testkeys.Private)

	k.Run()
}
