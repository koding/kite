package main

import (
	"github.com/koding/kite/config"
	"github.com/koding/kite/proxy"
	"github.com/koding/kite/testkeys"
)

func main() {
	p := proxy.New(testkeys.Public, testkeys.Private)
	p.Kite.Config = config.MustGet()
	p.ListenAndServe()
}
