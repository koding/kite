package server

import (
	"testing"

	"github.com/koding/kite"
)

func TestServerStart(t *testing.T) {
	k := kite.New("testkite", "1.0.0")

	k.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})

	server := New(k)
	server.Config.IP = "127.0.0.1"
	server.Config.Port = 3636
	server.Config.DisableAuthentication = true
	server.Start()

	client := kite.New("testclient", "1.0.0")
	remote := client.NewClientString("ws://localhost:3636")

	err := remote.Dial()
	if err != nil {
		t.Fatalf("cannot connect to remote kite: %s", err.Error())
	}

	result, err := remote.Tell("foo")
	if err != nil {
		t.Fatalf("cannot call method of remote kite: %s", err.Error())
	}

	answer := result.MustString()

	if answer != "bar" {
		t.Errorf("result = %s; want %s", answer, "bar")
	}
}
