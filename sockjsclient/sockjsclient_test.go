package sockjsclient_test

import (
	"net/url"
	"testing"

	"github.com/koding/kite/sockjsclient"
)

func TestMakeWebsocketURL(t *testing.T) {
	cases := map[string]string{
		"https://koding.com/kloud/kite":             "wss://koding.com:443/kloud/kite/server/session/websocket",
		"http://127.0.0.1:56789/kite":               "ws://127.0.0.1:56789/kite/server/session/websocket",
		"http://rjeczalik.koding.team/kontrol/kite": "ws://rjeczalik.koding.team:80/kontrol/kite/server/session/websocket",
	}

	for cas, want := range cases {
		u, err := url.Parse(cas)
		if err != nil {
			t.Fatalf("%s: Parse()=%s", cas, err)
		}

		got := sockjsclient.MakeWebsocketURL(u, "server", "session").String()

		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
