package main

import (
	"flag"
	"io/ioutil"
	"log"
	"strings"

	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
)

func main() {
	var (
		publicKeyFile  = flag.String("public-key", "", "")
		privateKeyFile = flag.String("private-key", "", "")
		ip             = flag.String("ip", "0.0.0.0", "")
		port           = flag.Int("port", 4000, "")
		name           = flag.String("name", "", "name of the instance")
		dataDir        = flag.String("data-dir", "", "directory to store data")
		peers          = flag.String("peers", "", "comma seperated peer addresses")
	)

	flag.Parse()

	if *publicKeyFile == "" {
		log.Fatalln("no -public-key given")
	}

	if *privateKeyFile == "" {
		log.Fatalln("no -private-key given")
	}

	publicKey, err := ioutil.ReadFile(*publicKeyFile)
	if err != nil {
		log.Fatalln("cannot read public key file")
	}

	privateKey, err := ioutil.ReadFile(*privateKeyFile)
	if err != nil {
		log.Fatalln("cannot read private key file")
	}

	conf := config.MustGet()
	conf.IP = *ip
	conf.Port = *port

	k := kontrol.New(conf, string(publicKey), string(privateKey))

	if *name != "" {
		k.Name = *name
	}
	if *dataDir != "" {
		k.DataDir = *dataDir
	}
	if *peers != "" {
		k.Peers = strings.Split(*peers, ",")
	}

	k.Run()
}
