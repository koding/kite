package main

import (
	"flag"
	"io/ioutil"
	"log"

	"github.com/koding/kite/config"
	"github.com/koding/kite/proxy"
)

func main() {
	var (
		publicKeyFile  = flag.String("public-key", "", "")
		privateKeyFile = flag.String("private-key", "", "")
		ip             = flag.String("ip", "0.0.0.0", "")
		port           = flag.Int("port", 4000, "")
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

	p := proxy.New(conf, string(publicKey), string(privateKey))

	p.Run()
}
