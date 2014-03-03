package main

import (
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/testutil"
)

func main() {
	kitekey.Write(testutil.NewKiteKey().Raw)
}
