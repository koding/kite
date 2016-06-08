package dnode

import "testing"

func TestUnmarshalArguments(t *testing.T) {
	arguments := &Partial{Raw: []byte(`["hello", "world"]`)}

	var s []string

	arguments.MustUnmarshal(&s)

	if len(s) != 2 {
		t.Errorf("Invalid array length: %d", len(s))
		return
	}

	if s[0] != "hello" || s[1] != "world" {
		t.Errorf("Invalid array")
		return
	}
}
