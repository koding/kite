package dnode

import "testing"

func TestScrubUnscrub(t *testing.T) {
	scrubber := NewScrubber()

	type Args struct {
		A int
		B string
		C Function
	}

	obj := Args{
		A: 1,
		B: "foo",
		C: Callback(func(*Partial) {}),
	}

	callbacks := scrubber.Scrub(obj)
	t.Logf("callbacks: %+q\n", callbacks)

	args := Args{
		A: 2,
		B: "bar",
	}

	var sent bool
	sendf := func(id uint64) functionReceived {
		return func(args ...interface{}) error {
			sent = true
			return nil
		}
	}

	scrubber.Unscrub(&args, callbacks, sendf)
	t.Logf("args: %+v\n", args)

	if args.C.Caller == nil {
		t.Fatal("callback is not set")
	}

	if err := args.C.Call(); err != nil {
		t.Error(err)
	}

	if !sent {
		t.Error("callback is not called")
	}
}
