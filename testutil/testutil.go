package testutil

import (
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"kite"
	"kite/kontrol"
	"kite/regserv"
	"kite/testkeys"
)

var (
	Kontrol *kontrol.Kontrol
	RegServ *regserv.RegServ
)

func init() {
	initKontrol()
	initRegServ()
	clearEtcd()
}

func initKontrol() {
	kontrolOptions := &kite.Options{
		Kitename:    "kontrol",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "testing",
		PublicIP:    "127.0.0.1",
		Port:        "3999",
		Path:        "/kontrol",
	}
	Kontrol = kontrol.New(kontrolOptions, nil, testkeys.Public, testkeys.Private)
}

func initRegServ() {
	backend := &testBackend{}
	RegServ = regserv.New(backend)
	RegServ.Environment = "testing"
	RegServ.Region = "localhost"
	RegServ.PublicIP = "127.0.0.1"
	RegServ.Port = "8079"
}

func clearEtcd() {
	etcdClient := etcd.NewClient(nil)
	_, err := etcdClient.Delete("/kites", true)
	if err != nil && err.(*etcd.EtcdError).ErrorCode != 100 { // Key Not Found
		panic(fmt.Errorf("Cannot delete keys from etcd: %s", err))
	}
}

type testBackend struct{}

func (b *testBackend) Issuer() string     { return "testuser" }
func (b *testBackend) KontrolURL() string { return "ws://localhost:3999/kontrol" }
func (b *testBackend) PublicKey() string  { return testkeys.Public }
func (b *testBackend) PrivateKey() string { return testkeys.Private }

func (b *testBackend) Authenticate(r *kite.Request) (string, error) {
	return "testuser", nil
}
