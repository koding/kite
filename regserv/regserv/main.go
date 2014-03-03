package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"

	"github.com/koding/kite/config"
	"github.com/koding/kite/regserv"
)

func main() {
	// Server options
	var (
		ip   = flag.String("ip", "0.0.0.0", "")
		port = flag.Int("port", 3998, "")
	)

	// Registration options
	var (
		init           = flag.Bool("init", false, "create a new kite.key")
		username       = flag.String("username", "", "")
		kontrolURL     = flag.String("kontrol-url", "", "")
		publicKeyFile  = flag.String("public-key", "", "")
		privateKeyFile = flag.String("private-key", "", "")
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

	if *init {
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

		s := regserv.New(conf, string(publicKey), string(privateKey))
		err = s.RegisterSelf()
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("kite.key is written to ~/.kite/kite.key. You can see it with:\n\tkite showkey")
		os.Exit(0)
	}

	conf, err := config.Get()
	if err != nil {
		fmt.Println(err)
		fmt.Println(noKeyMessage)
		os.Exit(1)
	}

	conf.IP = *ip
	conf.Port = *port

	s := regserv.New(conf, string(publicKey), string(privateKey))

	// Request must not be authenticated because clients do not have a
	// kite.key before they register. We will authenticate them in
	// "register" method handler.
	s.Server.Config.DisableAuthentication = true

	s.Run()
}

const noKeyMessage = `kite.key not found in ~/.kite/kite.key. Please register yourself with:
	regserv -init -username=<username> -kontrol-url=<url> -public-key=<filename> -private-key=<filename>
A new pair of keys can be created with:
	openssl genrsa -out privateKey.pem 2048
	openssl rsa -in privateKey.pem -pubout > publicKey.pem`
