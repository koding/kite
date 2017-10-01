package kontrol

import (
	"testing"

	"github.com/koding/kite"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/kontrol/util"
	"github.com/koding/kite/protocol"
)

func NewTestCrate(t *testing.T) (c util.TestContainer, s Storage) {
	if testing.Short() {
		t.Skip("Crate tests take a while to run")
	}
	log, _ := kite.NewLogger("test")
	c = util.StartDockerCrate(t)
	s = NewCrate(&CrateConfig{
		Host:  c.IPAddress(),
		Port:  4200,
		Table: "test_table",
	}, log)
	return c, s
}

func TestCrateAdd(t *testing.T) {
	c, s := NewTestCrate(t)
	defer c.Stop(t)
	storageAdd(s, t)
}

func TestCrateGet(t *testing.T) {
	c, s := NewTestCrate(t)
	defer c.Stop(t)
	storageGet(s, t)
}
func TestCrateDelete(t *testing.T) {
	c, s := NewTestCrate(t)
	defer c.Stop(t)
	storageDelete(s, t)
}

func storageAdd(s Storage, t *testing.T) {
	err := s.Add(&protocol.Kite{}, &kontrolprotocol.RegisterValue{})
	if err != nil {
		t.Fatal(err)
	}
}

func storageGet(s Storage, t *testing.T) {
	keyID := "test_key_id"
	kite := &protocol.Kite{ID: keyID}
	err := s.Add(kite, &kontrolprotocol.RegisterValue{})
	if err != nil {
		t.Fatal(err)
	}
	kites, err := s.Get(kite.Query())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range kites {
		if v.Kite.ID != keyID {
			t.Fatalf("Get returned incorrect keys '%s' != '%s'",
				keyID, v.KeyID)
		}
	}
}

func storageDelete(s Storage, t *testing.T) {
	keyID := "test_key_id"
	kite := &protocol.Kite{ID: keyID}
	err := s.Add(kite, &kontrolprotocol.RegisterValue{})
	if err != nil {
		t.Fatal(err)
	}
	kites, err := s.Get(kite.Query())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range kites {
		if v.Kite.ID != keyID {
			t.Fatalf("Get returned incorrect keys '%s' != '%s'",
				keyID, v.KeyID)
		}
	}
	err = s.Delete(kite)
	if err != nil {
		t.Fatal(err)
	}
	kites, err = s.Get(kite.Query())
	if err != nil {
		t.Fatal(err)
	}
	for range kites {
		t.Fatalf("Should have returned no kites!")
	}
}
