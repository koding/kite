package proxy

import (
	"kite"
	"kite/kontrol"
	"kite/protocol"
	"kite/testkeys"
	"kite/testutil"
	"net"
	"os"
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
	k := New(proxyOptions, "localhost", 8443, testkeys.Cert, testkeys.Key)
	k.Start()

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
	_, URLport, _ := net.SplitHostPort(kite1remote.Kite.URL.Host)
	if URLport != "8443" {
		t.Errorf("Wrong port: %s", URLport)
		return
	}

	err = kite1remote.Dial()
	if err != nil {
		t.Error(err.Error())
		// time.Sleep(time.Minute)
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
