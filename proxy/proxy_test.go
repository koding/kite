package proxy

import (
	"github.com/koding/kite"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
	"os"
	"strings"
	"testing"
	"time"
)

func setupTest() {
	testutil.WriteKiteKey()
}

func TestTLSKite(t *testing.T) {
	setupTest()

	opts := &kite.Options{
		Kitename:    "kontrol",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "testing",
		PublicIP:    "127.0.0.1",
		Port:        "3999",
		Path:        "/kontrol",
	}
	kon := kontrol.New(opts, "kontrol", os.TempDir(), nil, testkeys.Public, testkeys.Private)
	kon.Start()
	kon.ClearKites()

	// Kontrol is ready.

	proxyOptions := &kite.Options{
		Kitename:    "proxy",
		Version:     "0.0.1",
		Environment: "testing",
		Region:      "localhost",
	}
	k := New(proxyOptions, "localhost", testkeys.Cert, testkeys.Key, testkeys.Public, testkeys.Private)
	go k.ListenAndServe()

	// TLS Kite is ready.

	// Wait for it to register itself.
	time.Sleep(1000 * time.Millisecond)

	opt1 := &kite.Options{
		Kitename:    "kite1",
		Version:     "0.0.1",
		Environment: "testing",
		Region:      "localhost",
	}
	kite1 := kite.New(opt1)
	kite1.EnableProxy("testuser")
	kite1.AddRootCertificate(testkeys.Cert)
	kite1.HandleFunc("foo", func(r *kite.Request) (interface{}, error) {
		return "bar", nil
	})
	kite1.Start()
	defer kite1.Close()

	// kite1 is registered to Kontrol with address of TLS Kite.

	opt2 := &kite.Options{
		Kitename:    "kite2",
		Version:     "0.0.1",
		Environment: "testing",
		Region:      "localhost",
	}
	kite2 := kite.New(opt2)
	kite2.AddRootCertificate(testkeys.Cert)
	kite2.Start()
	defer kite2.Close()

	// kite2 is started.

	// Wait for kites to register to Kontrol.
	// TODO do not sleep, make a notifier method.
	time.Sleep(1000 * time.Millisecond)

	// Get the list of "kite1" kites from Kontrol.
	query := protocol.KontrolQuery{
		Username:    kite2.Username,
		Environment: "testing",
		Name:        "kite1",
	}
	kites, err := kite2.Kontrol.GetKites(query)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// Got kites from Kontrol.
	kite1remote := kites[0]

	// Check URL has the correct port number (TLS Kite's port).
	if !strings.HasPrefix(kite1remote.Kite.URL.Path, "/proxy") {
		t.Errorf("Invalid proxy URL: %s", kite1remote.Kite.URL.String())
		return
	}

	err = kite1remote.Dial()
	if err != nil {
		t.Error(err.Error())
		return
	}

	// kite2 is connected to kite1 via TLS kite.

	result, err := kite1remote.Tell("foo")
	if err != nil {
		t.Error(err.Error())
		return
	}

	s := result.MustString()
	if s != "bar" {
		t.Errorf("Wrong reply: %s", s)
		return
	}
}
