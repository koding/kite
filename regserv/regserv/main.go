package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"

	"github.com/koding/kite/config"
	"github.com/koding/kite/regserv"
	"github.com/koding/kite/testkeys"
)

func main() {
	// Server options
	var (
		environment = flag.String("environment", "unknown", "")
		region      = flag.String("region", "unknown", "")
		ip          = flag.String("ip", "0.0.0.0", "")
		portStr     = flag.String("port", "3998", "")
	)

	// Registration options
	var (
		registerSelf   = flag.Bool("register-self", false, "create a new kite.key")
		username       = flag.String("username", "", "")
		kontrolURL     = flag.String("kontrol-url", "", "")
		publicKeyFile  = flag.String("public-key", "", "")
		privateKeyFile = flag.String("private-key", "", "")
	)

	flag.Parse()

	if *registerSelf {
		conf := config.New()

		if *username == "" {
			log.Fatalln("empty username")
		}
		conf.Username = *username

		parsed, err := url.Parse(*kontrolURL)
		if err != nil {
			log.Fatalln("cannot parse kontrol URL")
		}
		conf.KontrolURL = parsed

		publicKey, err := ioutil.ReadFile(*publicKeyFile)
		if err != nil {
			log.Fatalln("cannot read public key file")
		}

		privateKey, err := ioutil.ReadFile(*privateKeyFile)
		if err != nil {
			log.Fatalln("cannot read private key file")
		}

		s := regserv.New(conf, string(publicKey), string(privateKey))
		err = s.RegisterSelf()
		if err != nil {
			log.Fatal(err)
		}
	}

	conf, err := config.Get()
	if err != nil {
		log.Fatalf("kite.key not found. Please register yourself with:\n\tregserv -register-self -username=<username> -kontrol-url=<url> -public-key=<filename> -private-key=<filename>\n")
	}

	conf.Environment = *environment
	conf.Region = *region
	conf.IP = *ip
	conf.Port, err = strconv.Atoi(*portStr)
	if err != nil {
		log.Fatalln("invalid port number")
	}

	s := regserv.New(conf, testkeys.Public, testkeys.Private)
	s.Run()
}
