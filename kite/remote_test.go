package kite

import (
	"testing"
	"time"
)

func TestSyncCall(t *testing.T) {
	options := &protocol.Options{
		Kitename: "adder",
		Version:  "1",
		Port:     "5000",
	}

	k := kite.New(options)
	k.RegisterMethod("add", func(a, b float64) float64 { return a + b })
	k.Start()

	sleep()

	r := NewRemoteKite(k.Kite, "")
	err := r.Dial()
	if err != nil {
		t.Error(err)
		return
	}

	result, err := r.Call("add", 1, 2)
	if err != nil {
		t.Error(err)
		return
	}

	i := result.(float64)
	if i != 3 {
		t.Errorf("Invalid result: %s", i)
	}
}

func sleep() { time.Sleep(100 * time.Millisecond) }
