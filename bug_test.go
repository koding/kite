package kite_test

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/koding/kite"
)

func TestKite_MultipleDial(t *testing.T) {
	esrv := kite.New("echo-server", "0.0.0")
	esrv.Config.DisableAuthentication = true
	if err := esrv.Config.ReadEnvironmentVariables(); err != nil {
		t.Fatal(err)
	}

	esrv.HandleFunc("echo", func(r *kite.Request) (interface{}, error) {
		var arg string

		if err := r.Args.One().Unmarshal(&arg); err != nil {
			return nil, err
		}

		if arg == "" {
			return nil, errors.New("arg is empty")
		}

		return arg, nil
	})

	ts := httptest.NewServer(esrv)
	ecli := kite.New("echo-client", "0.0.0")
	if err := ecli.Config.ReadEnvironmentVariables(); err != nil {
		t.Fatal(err)
	}

	esrv.SetLogLevel(kite.DEBUG)
	ecli.SetLogLevel(kite.DEBUG)

	c := ecli.NewClient(fmt.Sprintf("%s/kite", ts.URL))

	if err := c.Dial(); err != nil {
		t.Fatalf("dialing echo-server kite error: %s", err)
	}

	if err := c.Dial(); err != nil {
		t.Fatalf("dialing echo-server kite error: %s", err)
	}

	resp, err := c.Tell("echo", "Hello world!")
	if err != nil {
		t.Fatalf("Tell()=%s", err)
	}

	var reply string

	if err := resp.Unmarshal(&reply); err != nil {
		t.Fatalf("Unmarshal()=%s", err)
	}

	if reply != "Hello world!" {
		t.Fatalf(`got %q, want "Hello world!"`, reply)
	}
}
