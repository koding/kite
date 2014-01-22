// Echo kite is an example to kite that just echos the argument back
// Useful for debugging or testing stuff
package main

import (
	"fmt"
	"koding/newkite/kite"
	"koding/tools/config"
	"net/url"
	"strconv"
)

func main() {
	k := newKite()
	k.HandleFunc("echo", func(r *kite.Request) (interface{}, error) {
		return r.Args.One().MustString(), nil
	})

	k.Run()
}

func newKite() *kite.Kite {
	kontrolPort := strconv.Itoa(config.Current.NewKontrol.Port)
	kontrolHost := config.Current.NewKontrol.Host
	kontrolURL := &url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%s", kontrolHost, kontrolPort),
		Path:   "/dnode",
	}

	options := &kite.Options{
		Kitename:    "echo",
		Environment: config.FileProfile,
		Region:      config.Region,
		Version:     "0.0.1",
		KontrolURL:  kontrolURL,
	}

	return kite.New(options)
}
