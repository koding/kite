package kite

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"strings"
)

// Options is passed to kite.New when creating new instance.
type Options struct {
	// Mandatory fields
	Kitename    string
	Version     string
	Environment string
	Region      string

	// Optional fields
	PublicIP string // default: 0.0.0.0
	Port     string // default: random
	Path     string // default: /<kitename>

	// Do not authenticate incoming requests
	DisableAuthentication bool
}

// validate fields of the options struct. It exits if an error is occured.
func (o *Options) validate() {
	if o.Kitename == "" {
		log.Fatal("ERROR: options.Kitename field is not set")
	}

	if digits := strings.Split(o.Version, "."); len(digits) != 3 {
		log.Fatal("ERROR: please use 3-digits semantic versioning for options.version")
	}

	if o.Region == "" {
		log.Fatal("ERROR: options.Region field is not set")
	}

	if o.Environment == "" {
		log.Fatal("ERROR: options.Environment field is not set")
	}

	if o.PublicIP == "" {
		o.PublicIP = "0.0.0.0"
	}

	if o.Port == "" {
		o.Port = "0" // OS binds to an automatic port
	}

	if o.Path == "" {
		o.Path = "/kite/" + o.Kitename
	}

	if o.Path[0] != '/' {
		o.Path = "/" + o.Path
	}
}

// Read options from a file.
func ReadKiteOptions(configfile string) (*Options, error) {
	file, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, err
	}

	options := &Options{}
	return options, json.Unmarshal(file, &options)
}
