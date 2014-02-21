package main

import (
	"flag"
	"github.com/koding/kite"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/testkeys"
	"log"
	"os"
	"strings"
)

func main() {
	var name, dataDir, peersString string
	var peers []string

	options := &kite.Options{
		Kitename: "kontrol",
		Version:  "0.0.1",
		Path:     "/kontrol",
	}

	flag.StringVar(&options.Environment, "environment", "development", "")
	flag.StringVar(&options.Region, "region", "localhost", "")
	flag.StringVar(&options.PublicIP, "ip", "0.0.0.0", "")
	flag.StringVar(&options.Port, "port", "4000", "")

	flag.StringVar(&name, "name", "", "name of the instance")
	flag.StringVar(&dataDir, "data-dir", "", "directory to store data")
	flag.StringVar(&peersString, "peers", "", "comma seperated peer addresses")

	flag.Parse()

	if name == "" {
		var err error
		name, err = os.Hostname()
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	if dataDir == "" {
		log.Fatal("data-dir flag is not set")
	}

	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}

	k := kontrol.New(options, name, dataDir, peers, testkeys.Public, testkeys.Private)

	k.Run()
}
