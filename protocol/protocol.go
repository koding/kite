// Package protocol defines the communication between the components
// of the Kite infrastructure. It defines some constants and structures
// designed to be sent between those components.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/koding/kite/dnode"
)

// Kite is the base struct containing the public fields. It is usually embeded
// in other structs, including the db model. The access model is in the form:
// username.environment.name.version.region.hostname.id
type Kite struct {
	// Short name identifying the type of the kite. Example: fs, terminal...
	Name string `json:"name"`

	// Owner of the Kite
	Username string `json:"username"`

	// Every Kite instance has different identifier.
	// If a kite is restarted, it's id will change.
	// This is generated on the Kite.
	ID string `json:"id"`

	// Environment is defines as something like "production", "testing",
	// "staging" or whatever.  This allows you to differentiate between a
	// cluster of kites.
	Environment string `json:"environment"`

	// Region of the kite it is running. Like "Europe", "Asia" or some other
	// locations.
	Region string `json:"region"`

	// 3-digit semantic version.
	Version string `json:"version"`

	// os.Hostname() of the Kite.
	Hostname string `json:"hostname"`
}

func (k Kite) String() string {
	return "/" + k.Username + "/" + k.Environment + "/" + k.Name + "/" + k.Version + "/" + k.Region + "/" + k.Hostname + "/" + k.ID
}

func (k *Kite) Validate() error {
	s := k.String()
	if strings.Contains(s, "//") {
		return errors.New("empty field")
	}
	if strings.Count(s, "/") != 7 {
		return errors.New(`fields cannot contain "/"`)
	}
	return nil
}

// KiteFromString returns a new Kite struct from the given string
// representation. It's the inverse of k.String()
func KiteFromString(stringRepr string) (*Kite, error) {
	fields := strings.Split(strings.TrimPrefix(stringRepr, "/"), "/")

	if len(fields) != 7 {
		return nil, fmt.Errorf("invalid kite string: %s", stringRepr)
	}

	return &Kite{
		Username:    fields[0],
		Environment: fields[1],
		Name:        fields[2],
		Version:     fields[3],
		Region:      fields[4],
		Hostname:    fields[5],
		ID:          fields[6],
	}, nil

}

// RegisterArgs is used as the function argument to the Kontrol's register
// method.
type RegisterArgs struct {
	URL string `json:"url"`
}

// RegisterResult is a response to Register request from Kite to Kontrol.
type RegisterResult struct {
	URL string `json:"url"`
}

type GetKitesArgs struct {
	Query         KontrolQuery    `json:"query"`
	WatchCallback dnode.Function  `json:"watchCallback"`
	Who           json.RawMessage `json:"who"`
}

type WhoResult struct {
	Query KontrolQuery `json:"query"`
}

type GetKitesResult struct {
	Kites     []*KiteWithToken `json:"kites"`
	WatcherID string           `json:"watcherID,omitempty"`
}

type KiteWithToken struct {
	Kite  Kite   `json:"kite"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

// KiteEvent is the struct that is sent as an argument in watchCallback of
// getKites method of Kontrol.
type KiteEvent struct {
	Action KiteAction `json:"action"`
	Kite   Kite       `json:"kite"`

	// Required to connect when Action == Register
	URL   string `json:"url,omitempty"`
	Token string `json:"token,omitempty"`
}

type KiteAction string

const (
	Register   KiteAction = "REGISTER"
	Deregister KiteAction = "DEREGISTER"
)

// KontrolQuery is a structure of message sent to Kontrol. It is used for
// querying kites based on the incoming field parameters. Missing fields are
// not counted during the query (for example if the "version" field is empty,
// any kite with different version is going to be matched).
// Order of the fields is from general to specific.
type KontrolQuery struct {
	Username    string `json:"username"`
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Region      string `json:"region"`
	Hostname    string `json:"hostname"`
	ID          string `json:"id"`
}
