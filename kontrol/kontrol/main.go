package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"

	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/multiconfig"
)

type Kontrol struct {
	Ip          string
	Port        int
	TLSCertFile string
	TLSKeyFile  string
	RegisterUrl string

	Initial    bool
	Username   string
	KontrolURL string

	PublicKeyFile  string `required:"true"`
	PrivateKeyFile string `required:"true"`

	Machines []string
	Version  string

	Postgres struct {
		Host     string `default:"localhost"`
		Port     int    `default:"5432"`
		Username string `required:"true"`
		Password string
		DBName   string `required:"true" `
	}
}

var (
	// Kite server options
	ip          = flag.String("ip", "0.0.0.0", "")
	port        = flag.Int("port", 4000, "")
	tlsCertFile = flag.String("tls-cert", "", "TLS certificate file")
	tlsKeyFile  = flag.String("tls-key", "", "TLS key file")
	registerURL = flag.String("register-url", "", "Change self register URL")

	// For self register and initial first key on a machine
	initial    = flag.Bool("init", false, "create a new kite.key")
	username   = flag.String("username", "", "")
	kontrolURL = flag.String("kontrol-url", "", "")

	// For signing/validating tokens
	publicKeyFile  = flag.String("public-key", "", "Public RSA key")
	privateKeyFile = flag.String("private-key", "", "Private RSA key")

	// etcd instance options
	machines = flag.String("machines", "", "comma seperated peer addresses")

	version = flag.String("version", "0.0.1", "version of kontrol")
)

func main() {
	conf := new(Kontrol)

	multiconfig.New().MustLoad(conf)

	publicKey, err := ioutil.ReadFile(conf.PublicKeyFile)
	if err != nil {
		log.Fatalf("cannot read public key file: %s", err.Error())
	}

	privateKey, err := ioutil.ReadFile(conf.PrivateKeyFile)
	if err != nil {
		log.Fatalf("cannot read private key file: %s", err.Error())
	}

	if conf.Initial {
		initialKey(publicKey, privateKey)
		os.Exit(0)
	}

	kiteConf := config.MustGet()
	kiteConf.IP = conf.Ip
	kiteConf.Port = conf.Port

	k := kontrol.New(kiteConf, conf.Version, string(publicKey), string(privateKey))

	if conf.TLSCertFile != "" || conf.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.TLSCertFile, conf.TLSKeyFile)
		if err != nil {
			log.Fatalf("cannot load TLS certificate: %s", err.Error())
		}

		k.Kite.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	if conf.RegisterUrl != "" {
		k.RegisterURL = conf.RegisterUrl
	}

	switch os.Getenv("KONTROL_STORAGE") {
	case "etcd":
		k.SetStorage(kontrol.NewEtcd(conf.Machines, k.Kite.Log))
	case "postgres":
		postgresConf := &kontrol.PostgresConfig{
			Host:     conf.Postgres.Host,
			Port:     conf.Postgres.Port,
			Username: conf.Postgres.Username,
			Password: conf.Postgres.Password,
			DBName:   conf.Postgres.DBName,
		}

		k.SetStorage(kontrol.NewPostgres(postgresConf, k.Kite.Log))
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
	err = k.InitializeSelf()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("kite.key is written to ~/.kite/kite.key. You can see it with:\n\tkite showkey")
}
