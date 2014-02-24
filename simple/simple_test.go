package simple

import (
	"testing"
	"time"

	"github.com/koding/kite"
	"github.com/koding/kite/kontrol"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/testkeys"
)

func TestSimple(t *testing.T) {
	kon := kontrol.New(testkeys.Public, testkeys.Private)
	kon.Start()

	time.Sleep(1e9)

	p := proxy.New(testkeys.Public, testkeys.Private)
	go p.ListenAndServe()

	time.Sleep(1e9)

	s := New("hello", "1.0.0")
	s.HandleFunc("hello", hello2)
	s.Run()
}

func hello2(r *kite.Request) (interface{}, error) {
	return "hello", nil
}
