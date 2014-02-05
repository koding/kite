package main

import (
	"kite"
	"kite/kontrol"
	"kite/testkeys"
)

func main() {
	options := &kite.Options{
		Kitename:    "kontrol",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "production",
		PublicIP:    "127.0.0.1",
		Port:        "4000",
		Path:        "/kontrol",
	}
	k := kontrol.New(options, nil, "example.com", testkeys.Public, testkeys.Private)
	k.Run()
}
