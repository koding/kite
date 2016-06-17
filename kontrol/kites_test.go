package kontrol_test

import (
	"reflect"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
)

func TestKitesShuffle(t *testing.T) {
	kites := kontrol.Kites{
		{KeyID: "1"},
		{KeyID: "2"},
		{KeyID: "3"},
		{KeyID: "4"},
		{KeyID: "5"},
		{KeyID: "6"},
		{KeyID: "7"},
		{KeyID: "8"},
		{KeyID: "9"},
	}

	kitesCopy := make(kontrol.Kites, len(kites))
	copy(kitesCopy, kites)

	kites.Shuffle()

	if reflect.DeepEqual(kites, kitesCopy) {
		t.Fatal("wanted kites to be shuffled")
	}
}

func TestKitesFilter(t *testing.T) {
	kites := kontrol.Kites{
		{Kite: protocol.Kite{Version: "1.0.0"}},
		{Kite: protocol.Kite{Version: "1.1.0"}},
		{Kite: protocol.Kite{Version: "1.2.0"}},
		{Kite: protocol.Kite{Version: "1.3.0"}},
		{Kite: protocol.Kite{Version: "1.4.0"}},
		{Kite: protocol.Kite{Version: "1.5.0"}},
		{Kite: protocol.Kite{Version: "1.6.0"}},
		{Kite: protocol.Kite{Version: "1.7.0"}},
		{Kite: protocol.Kite{Version: "1.8.0"}},
		{Kite: protocol.Kite{Version: "1.9.0"}},
	}

	want := kontrol.Kites{
		kites[6],
		kites[7],
		kites[8],
		kites[9],
	}

	c, err := version.NewConstraint(">= 1.5.5")
	if err != nil {
		t.Fatal(err)
	}

	kites.Filter(c, "")

	if !reflect.DeepEqual(kites, want) {
		t.Fatalf("got %+v, want %+v", kites, want)
	}
}
