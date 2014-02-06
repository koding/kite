package main

import (
	"flag"
	"kite"
	"kite/regserv"
	"kite/testkeys"
)

func main() {
	backend := &exampleBackend{}

	server := regserv.New(backend)

	flag.StringVar(&server.Environment, "environment", "development", "")
	flag.StringVar(&server.Region, "region", "localhost", "")
	flag.StringVar(&server.PublicIP, "ip", "0.0.0.0", "")
	flag.StringVar(&server.Port, "port", "8080", "")

	flag.Parse()

	server.Run()
}

type exampleBackend struct{}

func (b *exampleBackend) Issuer() string     { return "testuser" }
func (b *exampleBackend) KontrolURL() string { return "ws://localhost:4000/kontrol" }
func (b *exampleBackend) PublicKey() string  { return testkeys.Public }
func (b *exampleBackend) PrivateKey() string { return testkeys.Private }

func (b *exampleBackend) Authenticate(r *kite.Request) (string, error) {
	return "testuser", nil
}
