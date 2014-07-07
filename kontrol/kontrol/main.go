package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
)

var (
	// Kite server options
	ip          = flag.String("ip", "0.0.0.0", "")
	port        = flag.Int("port", 4000, "")
	tlsCertFile = flag.String("tls-cert", "", "TLS certificate file")
	tlsKeyFile  = flag.String("tls-key", "", "TLS key file")

	// For self register and initial first key on a machine
	initial    = flag.Bool("init", false, "create a new kite.key")
	username   = flag.String("username", "", "")
	kontrolURL = flag.String("kontrol-url", "", "")

	// For signing/validating tokens
	publicKeyFile  = flag.String("public-key", "", "Public RSA key")
	privateKeyFile = flag.String("private-key", "", "Private RSA key")

	// etcd instance options
	name         = flag.String("name", "", "name of the instance")
	dataDir      = flag.String("data-dir", "", "directory to store data")
	etcdAddr     = flag.String("etcd-addr", "127.0.0.1:4001", "The public host:port used for etcd server.")
	etcdBindAddr = flag.String("etcd-bind-addr", "0.0.0.0:4001", "The listening host:port used for etcd server.")
	peerAddr     = flag.String("peer-addr", "127.0.0.1:7001", "The public host:port used for peer communication.")
	peerBindAddr = flag.String("peer-bind-addr", "0.0.0.0:7001", "The listening host:port used for peer communication.")
	peers        = flag.String("peers", "", "comma seperated peer addresses")
	version      = flag.String("version", "0.0.1", "version of kontrol")
)

func main() {
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

	if *initial {
		initialKey(publicKey, privateKey)
		os.Exit(0)
	}

	conf := config.MustGet()
	conf.IP = *ip
	conf.Port = *port

	k := kontrol.New(conf, *version, string(publicKey), string(privateKey))
	k.EtcdAddr = *etcdAddr
	k.EtcdBindAddr = *etcdBindAddr
	k.PeerAddr = *peerAddr
	k.PeerBindAddr = *peerBindAddr

	if *tlsCertFile != "" || *tlsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(*tlsCertFile, *tlsKeyFile)
		if err != nil {
			log.Fatalf("cannot load TLS certificate: %s", err.Error())
		}

		k.Kite.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
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

func initialKey(publicKey, privateKey []byte) {
	conf := config.New()

	if *username == "" {
		log.Fatalln("empty username")
	}
	conf.Username = *username

	_, err := url.Parse(*kontrolURL)
	if err != nil {
		log.Fatalln("cannot parse kontrol URL")
	}

	conf.KontrolURL = *kontrolURL

	k := kontrol.New(conf, *version, string(publicKey), string(privateKey))
	err = k.RegisterSelf()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("kite.key is written to ~/.kite/kite.key. You can see it with:\n\tkite showkey")
}
