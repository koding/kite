package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/kontrol"
	"github.com/koding/multiconfig"
)

type Kontrol struct {
	Ip          string
	Storage     string `default:"etcd"`
	Port        int
	TLSCertFile string
	TLSKeyFile  string
	RegisterUrl string

	Timeout    time.Duration `default:"30s"`
	Initial    bool
	Username   string
	KontrolURL string

	PublicKeyFile  string
	PrivateKeyFile string

	Machines []string
	Version  string `default:"0.0.1"`

	Postgres struct {
		Host           string `default:"localhost"`
		Port           int    `default:"5432"`
		Username       string
		Password       string
		DBName         string
		ConnectTimeout int `default:"20"`
	}

	Crate struct {
		Host  string `default:"127.0.0.1"`
		Port  int    `default:"4200"`
		Table string `default:"kontrol"`
	}
}

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
		initialKey(conf, publicKey, privateKey)
		os.Exit(0)
	}

	kiteConf := config.MustGet()
	kiteConf.IP = conf.Ip
	kiteConf.Port = conf.Port

	k := kontrol.New(kiteConf, conf.Version)

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

	switch conf.Storage {
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

		p := kontrol.NewPostgres(postgresConf, k.Kite.Log)
		p.Wait(conf.Timeout)
		k.SetStorage(p)
		k.SetKeyPairStorage(p)
	case "crate":
		crateConf := &kontrol.CrateConfig{
			Host:  conf.Crate.Host,
			Port:  conf.Crate.Port,
			Table: conf.Crate.Table,
		}

		c := kontrol.NewCrate(crateConf, k.Kite.Log)
		c.Wait(conf.Timeout)
		k.SetStorage(c)
	}

	k.AddKeyPair("", string(publicKey), string(privateKey))
	k.Kite.SetLogLevel(kite.DEBUG)
	k.Run()
}

func initialKey(kontrolConf *Kontrol, publicKey, privateKey []byte) {
	conf := config.New()

	if kontrolConf.Username == "" {
		log.Fatalln("empty username")
	}
	conf.Username = kontrolConf.Username

	_, err := url.Parse(kontrolConf.KontrolURL)
	if err != nil {
		log.Fatalln("cannot parse kontrol URL")
	}

	conf.KontrolURL = kontrolConf.KontrolURL

	k := kontrol.New(conf, kontrolConf.Version)
	k.AddKeyPair("", string(publicKey), string(privateKey))
	err = k.InitializeSelf()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("kite.key is written to ~/.kite/kite.key. You can see it with:\n\tkitectl showkey")
}
