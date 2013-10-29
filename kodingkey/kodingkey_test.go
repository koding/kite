package kodingkey

import (
	"fmt"
	"testing"
)

func TestKodingKey(t *testing.T) {
	k := NewKodingKey()
	fmt.Println("Koding key:", k)

	if len(k.String()) != StringLength {
		t.Errorf("%s != %s", len(k.String()), StringLength)
		return
	}
}
