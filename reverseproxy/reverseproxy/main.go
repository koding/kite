package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/koding/kite/config"
	"github.com/koding/kite/reverseproxy"
)

var (
	flagCertFile    = flag.String("cert", "", "Cert file to be used for HTTPS")
	flagKeyFile     = flag.String("key", "", "Key file to be used for HTTPS")
	flagIp          = flag.String("ip", "0.0.0.0", "Listening IP")
	flagPort        = flag.Int("port", 3999, "Server port")
	flagPublicHost  = flag.String("host", "127.0.0.1:3999", "Public host of Proxy.")
	flagRegion      = flag.String("region", "", "Change region")
	flagEnvironment = flag.String("env", "development", "Change development")
	flagVersion     = flag.Bool("version", false, "Show version and exit")
)

func main() {
	flag.Parse()

	if *flagVersion {
		fmt.Println(reverseproxy.Version)
		os.Exit(0)
	}

	if *flagRegion == "" || *flagEnvironment == "" {
		log.Fatal("Please specify envrionment via -env and region via -region. Aborting.")
	}

	scheme := "ws"
	if *flagCertFile != "" && *flagKeyFile != "" {
		scheme = "wss"
	}

	conf := config.MustGet()
	conf.IP = *flagIp
	conf.Port = *flagPort
	conf.Region = *flagRegion
	conf.Environment = *flagEnvironment

	r := reverseproxy.New(conf)
	r.PublicHost = *flagPublicHost
	r.Scheme = scheme

	registerURL := &url.URL{
		Scheme: scheme,
		Host:   r.PublicHost + ":" + strconv.Itoa(*flagPort),
		Path:   "/kite",
	}

	r.Kite.Log.Info("Registering with register url %s", registerURL)
	if err := r.Kite.RegisterForever(registerURL); err != nil {
		r.Kite.Log.Fatal("Registering to Kontrol: %s", err)
	}

	if *flagCertFile == "" || *flagKeyFile == "" {
		log.Println("No cert/key files are defined. Running proxy unsecure.")
		err := r.ListenAndServe()
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	} else {
		err := r.ListenAndServeTLS(*flagCertFile, *flagKeyFile)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}
}
