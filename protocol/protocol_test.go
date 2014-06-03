package protocol

import "testing"

func TestKite(t *testing.T) {
	k := Kite{
		Name:        "name",
		Username:    "username",
		ID:          "id",
		Environment: "environment",
		Region:      "region",
		Version:     "version",
		Hostname:    "hostname",
	}

	s := k.String()

	if err := k.Validate(); err != nil {
		t.Error(err)
	}

	d, err := KiteFromString(s)
	if err != nil {
		t.Error(err)
	}

	if err := d.Validate(); err != nil {
		t.Error(err)
	}

	expect := func(expecting, got string) {
		if expecting != got {
			t.Errorf("expecting: '%s',  got: '%s'", expecting, got)
		}
	}

	expect(d.Name, "name")
	expect(d.Username, "username")
	expect(d.ID, "id")
	expect(d.Environment, "environment")
	expect(d.Region, "region")
	expect(d.Version, "version")
	expect(d.Hostname, "hostname")
}
