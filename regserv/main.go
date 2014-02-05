package main

import (
	"koding/kite"
	"koding/kite/regserv/regserv"
	"koding/kite/testkeys"
)

func main() {
	b := &exampleBackend{}
	s := regserv.New(b)
	s.Run()
}

type exampleBackend struct{}

func (b *exampleBackend) Issuer() string     { return "example.com" }
func (b *exampleBackend) KontrolURL() string { return "ws://localhost:4000/kontrol" }
func (b *exampleBackend) PublicKey() string  { return testkeys.Public }
func (b *exampleBackend) PrivateKey() string { return testkeys.Private }

func (b *exampleBackend) Authenticate(r *kite.Request) (string, error) {
	return "testuser", nil
}
