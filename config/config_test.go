package config_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/koding/kite/config"

	"github.com/igm/sockjs-go/sockjs"
)

func TestConfigCopy(t *testing.T) {
	cases := []*config.Config{
		config.DefaultConfig, {
			KontrolURL: "https://koding.com/kontrol/kite",
			Username:   "john",
		}, {
			Environment: "aws",
			XHR:         http.DefaultClient,
			SockJS:      &sockjs.DefaultOptions,
		},
	}

	for _, cas := range cases {
		copy := cas.Copy()

		if !reflect.DeepEqual(copy, cas) {
			t.Fatalf("got %#v, want %#v", copy, cas)
		}
	}
}
