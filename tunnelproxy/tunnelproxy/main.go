package main

import (
	"flag"
	"io/ioutil"
	"log"

	"github.com/koding/kite/config"
	"github.com/koding/kite/tunnelproxy"
)

func main() {
	var (
		publicKeyFile  = flag.String("public-key", "", "")
		privateKeyFile = flag.String("private-key", "", "")
		ip             = flag.String("ip", "0.0.0.0", "")
		port           = flag.Int("port", 3999, "")
		publicHost     = flag.String("public-host", "127.0.0.1:3999", "")
		version        = flag.String("version", "0.0.1", "")
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

	t := tunnelproxy.New(conf, *version, string(publicKey), string(privateKey))
	t.PublicHost = *publicHost

	t.Run()
}
