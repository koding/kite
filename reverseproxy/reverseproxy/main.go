package main

import (
	"flag"
	"log"

	"github.com/koding/kite/config"
	"github.com/koding/kite/reverseproxy"
)

var (
	flagCertFile   = flag.String("certFile", "", "Cert file to be used for HTTPS")
	flagKeyFile    = flag.String("keyFile", "", "Key file to be used for HTTPS")
	flagIp         = flag.String("ip", "0.0.0.0", "Listening IP")
	flagPort       = flag.Int("port", 3999, "Server port")
	flagPublicHost = flag.String("public-host", "127.0.0.1:3999", "Public host of Proxy.")
)

func main() {
	flag.Parse()

	conf := config.MustGet()
	conf.IP = *flagIp
	conf.Port = *flagPort

	r := reverseproxy.New(conf)
	r.PublicHost = *flagPublicHost
	r.Scheme = "ws"

	if *flagCertFile == "" || *flagKeyFile == "" {
		log.Println("No cert/key files are defined. Running proxy cleanly.")
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
