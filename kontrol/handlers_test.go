package kontrol

import (
	"fmt"
	"strings"
	"testing"

	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
)

// createTestKite creates a test kite, caller of this func should close the kite
func createTestKite(name string, conf *Config, t *testing.T) *HelloKite {
	k, err := NewHelloKite(name, conf)
	if err != nil {
		t.Fatalf("error creating %s: %s", name, err)
	}

	k.Kite.HandleFunc(kite.WebRTCHandlerName, func(req *kite.Request) (interface{}, error) {
		return nil, fmt.Errorf("%s is called", name)
	})

	return k
}

func TestKontrol_HandleWebRTC(t *testing.T) {
	kont, conf := startKontrol(testkeys.PrivateThird, testkeys.PublicThird, 5501)
	defer kont.Close()

	hk1 := createTestKite("kite1", conf, t)
	defer hk1.Close()

	hk2 := createTestKite("kite2", conf, t)
	defer hk2.Close()

	err := hk1.Kite.SendWebRTCRequest(&protocol.WebRTCSignalMessage{Dst: hk2.Kite.Id})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected kite.errDstNotRegistered, got: %+v", err)
	}

	err = hk2.Kite.SendWebRTCRequest(&protocol.WebRTCSignalMessage{Dst: hk1.Kite.Id})
	if !strings.Contains(err.Error(), fmt.Sprintf("%s is called", hk1.Kite.Kite().Name)) {
		t.Fatalf("expected hk1 error, got: %+v", err)
	}
}
