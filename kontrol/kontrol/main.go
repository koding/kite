package main

import (
	"crypto/tls"
	"flag"
	"io/ioutil"
	"log"
	"strings"

	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
)

func main() {
	var (
		publicKeyFile  = flag.String("public-key", "", "Public RSA key")
		privateKeyFile = flag.String("private-key", "", "Private RSA key")
		ip             = flag.String("ip", "0.0.0.0", "")
		port           = flag.Int("port", 4000, "")
		etcdAddr       = flag.String("etcd-addr", "http://127.0.0.1:4001", "The public host:port used for etcd server.")
		etcdBindAddr   = flag.String("etcd-bind-addr", ":4001", "The listening host:port used for etcd server.")
		peerAddr       = flag.String("peer-addr", "http://127.0.0.1:7001", "The public host:port used for peer communication.")
		peerBindAddr   = flag.String("peer-bind-addr", ":7001", "The listening host:port used for peer communication.")
		name           = flag.String("name", "", "name of the instance")
		dataDir        = flag.String("data-dir", "", "directory to store data")
		peers          = flag.String("peers", "", "comma seperated peer addresses")
		tlsCertFile    = flag.String("tls-cert", "", "TLS certificate file")
		tlsKeyFile     = flag.String("tls-key", "", "TLS key file")
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
		log.Fatalf("cannot read public key file: %s", err.Error())
	}

	privateKey, err := ioutil.ReadFile(*privateKeyFile)
	if err != nil {
		log.Fatalf("cannot read private key file: %s", err.Error())
	}

	conf := config.MustGet()
	conf.IP = *ip
	conf.Port = *port

	k := kontrol.New(conf, string(publicKey), string(privateKey))
	k.EtcdAddr = *etcdAddr
	k.EtcdBindAddr = *etcdBindAddr
	k.PeerAddr = *peerAddr
	k.PeerBindAddr = *peerBindAddr

	if *tlsCertFile != "" || *tlsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(*tlsCertFile, *tlsKeyFile)
		if err != nil {
			log.Fatalf("cannot load TLS certificate: %s", err.Error())
		}

		k.Server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

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
