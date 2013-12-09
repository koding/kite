package kite

import (
	"encoding/json"
	"io/ioutil"
	"koding/newkite/protocol"
	"log"
	"strings"
)

// Options is used to define a Kite.
type Options struct {
	Username     string
	Kitename     string
	LocalIP      string
	PublicIP     string
	Environment  string
	Region       string
	Port         string
	Version      string
	KontrolAddr  string
	Dependencies string
	Visibility   protocol.Visibility
}

// validate is validating the fields of the options struct. It exists if an
// error is occured.
func (o *Options) validate() {
	if o.Kitename == "" {
		log.Fatal("ERROR: options.Kitename field is not set")
	}

	if o.Region == "" {
		log.Fatal("ERROR: options.Region field is not set")
	}

	if o.Environment == "" {
		log.Fatal("ERROR: options.Environment field is not set")
	}

	if o.Port == "" {
		o.Port = "0" // OS binds to an automatic port
	}

	if digits := strings.Split(o.Version, "."); len(digits) != 3 {
		log.Fatal("ERROR: please use 3-digits semantic versioning for options.version")
	}

	if o.KontrolAddr == "" {
		o.KontrolAddr = "127.0.0.1:4000" // local fallback address
	}

	if o.Visibility == protocol.Visibility("") {
		o.Visibility = protocol.Private
	}
}

func ReadKiteOptions(configfile string) (*Options, error) {
	file, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, err
	}

	options := &Options{}
	err = json.Unmarshal(file, &options)
	if err != nil {
		return nil, err
	}

	return options, nil
}
