package kite

import (
	"testing"
)

func testSplitVersion(t *testing.T) {
	name, version, err := splitVersion("asdf-1.2.3")
	if err != nil {
		t.Error(err)
	}
	if name != "asdf" {
		t.Error("Name is not ok:", name)
	}
	if version != "1.2.3" {
		t.Error("Version is not ok:", version)
	}

	name, version, err = splitVersion("asdf")
	if err == nil {
		t.Error(err)
	}
}
