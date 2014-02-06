package main

import (
	"kite"
	"kite/protocol"
	"kites/kontrol"
	"net"
	"os"
	"testing"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/op/go-logging"
)

func setupTest(t *testing.T) {
	// Print kite name in front of log message.
	logging.SetFormatter(logging.MustStringFormatter("[%{module:-8s}] %{level:-8s} ▶ %{message}"))
	stderrBackend := logging.NewLogBackend(os.Stderr, "", 0)
	stderrBackend.Color = true
	logging.SetBackend(stderrBackend)

	// Clean etcd
	etcdClient := etcd.NewClient(nil)
	_, err := etcdClient.Delete("/kites", true)
	if err != nil {
		if err.(etcd.EtcdError).ErrorCode != 100 { // Key Not Found
			t.Errorf("Cannot delete keys from etcd: %s", err)
			return
		}
	}
}

func TestTLSKite(t *testing.T) {
	setupTest(t)

	kon := kontrol.New()
	kon.Start()

	// Kontrol is ready.

	k := New(8443)
	k.Start()

	// TLS Kite is ready.

	// Wait for it to register itself.
	time.Sleep(1000 * time.Millisecond)

	opt1 := &kite.Options{
		Kitename:    "kite1",
		Version:     "0.0.1",
		Environment: "development",
		Region:      "localhost",
	}
	kite1 := kite.New(opt1)
	kite1.EnableProxy()
	kite1.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})
	kite1.Start()
	defer kite1.Close()

	// kite1 is registered to Kontrol with address of TLS Kite.

	opt2 := &kite.Options{
		Kitename:    "kite2",
		Version:     "0.0.1",
		Environment: "development",
		Region:      "localhost",
	}
	kite2 := kite.New(opt2)
	kite2.Start()
	defer kite2.Close()

	// kite2 is started.

	// Wait for kites to register to Kontrol.
	// TODO do not sleep, make a notifier method.
	time.Sleep(1000 * time.Millisecond)

	// Get the list of "kite1" kites from Kontrol.
	query := protocol.KontrolQuery{
		Username:    "devrim",
		Environment: "development",
		Name:        "kite1",
	}
	kites, err := kite2.Kontrol.GetKites(query)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// Got kites from Kontrol.
	remote := kites[0]

	// Check URL has the correct port number (TLS Kite's port).
	_, URLport, _ := net.SplitHostPort(remote.Kite.URL.Host)
	if URLport != "8443" {
		t.Errorf("Wrong port: %s", URLport)
		return
	}

	err = remote.Dial()
	if err != nil {
		t.Error(err.Error())
		// time.Sleep(time.Minute)
		return
	}

	// kite2 is connected to kite1 via TLS kite.

	result, err := remote.Tell("foo")
	if err != nil {
		t.Error(err.Error())
		return
	}

	s := result.MustString()
	if s != "bar" {
		t.Errorf("Wrong reply: %s", s)
		return
	}
}
