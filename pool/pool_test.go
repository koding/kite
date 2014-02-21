package pool

import (
	"fmt"
	"github.com/koding/kite"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/testkeys"
	"github.com/koding/kite/testutil"
	"os"
	"testing"
	"time"
)

func TestPool(t *testing.T) {
	testutil.WriteKiteKey()

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

	optsFoo := &kite.Options{
		Kitename:    "foo",
		Version:     "0.0.1",
		Environment: "testing",
		Region:      "localhost",
	}
	foo := kite.New(optsFoo)
	foo.Start()
	defer foo.Close()
	time.Sleep(1e9)

	fmt.Println("--- starting pool")
	query := protocol.KontrolQuery{
		Username:    foo.Username,
		Environment: "testing",
		Name:        "bar",
	}
	p := New(foo.Kontrol, query)
	errChan := p.Start()
	go func() {
		err := <-errChan
		if err != nil {
			t.Fatalf(err.Error())
		}
	}()
	time.Sleep(1e9)

	if len(p.Kites) != 0 {
		t.Errorf("no kite expected")
		return
	}

	optsBar := &kite.Options{
		Kitename:    "bar",
		Version:     "0.0.1",
		Environment: "testing",
		Region:      "localhost",
	}
	bar := kite.New(optsBar)
	bar.Start()
	defer bar.Close()
	time.Sleep(1e9)

	if len(p.Kites) != 1 {
		t.Errorf("1 kite expected")
		return
	}
}
