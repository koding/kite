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

func storageAdd(s Storage, t *testing.T) {
	err := s.Add(&protocol.Kite{}, &kontrolprotocol.RegisterValue{})
	if err != nil {
		t.Fatal(err)
	}
}
