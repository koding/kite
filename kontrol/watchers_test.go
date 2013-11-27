package kontrol

import (
	"koding/newkite/protocol"
	"testing"
)

func TestQueryMatch(t *testing.T) {
	kite := &protocol.Kite{
		Name:     "foo",
		Username: "cenk",
		Version:  "1",
	}
	query1 := &protocol.KontrolQuery{
		Name: "foo",
	}
	query2 := &protocol.KontrolQuery{
		Name:     "foo",
		Username: "nil",
	}
	if !matches(kite, query1) {
		t.Error("Must match")
	}
	if matches(kite, query2) {
		t.Error("Must not match")
	}
}
