package dnode

import (
	"fmt"
	"testing"
)

func TestUnmarshalArguments(t *testing.T) {
	arguments := Arguments{
		&Partial{Raw: []byte(`"hello"`)},
		&Partial{Raw: []byte(`"world"`)},
	}

	var s []string

	arguments.MustUnmarshal(&s)

	fmt.Printf("s: %#v\n", s)

	if len(s) != 2 {
		t.Errorf("Invalid array length: %d", len(s))
		return
	}

	if s[0] != "hello" || s[1] != "world" {
		t.Errorf("Invalid array")
		return
	}
}
