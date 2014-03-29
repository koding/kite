package dnode

import (
	"fmt"
	"testing"
)

func TestScrubUnscrub(t *testing.T) {
	scrubber := NewScrubber()

	type Args struct {
		A int
		B string
		C Callback
	}

	var called bool

	obj := Args{
		A: 1,
		B: "foo",
		C: Callback(func(Arguments) {
			called = true
		}),
	}

	callbacks := scrubber.Scrub(obj)
	fmt.Printf("--- callbacks: %+q\n", callbacks)

	args := Args{
		A: 2,
		B: "bar",
	}

	scrubber.Unscrub(&args, callbacks, scrubber.GetCallback)
	fmt.Printf("--- args: %+v\n", args)

	args.C(nil)

	if !called {
		t.Error("callback is not called")
	}
}
