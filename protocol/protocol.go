// Package protocol defines the communication between the components
// of the Kite infrastructure. It defines some constants and structures
// designed to be sent between those components.
package protocol

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

	// The URL that the Kite can be reached from. Marshaled as string.
	URL *KiteURL `json:"url" dnode:"-"`
}

func (k *Kite) Key() string {
	return "/" + k.Username + "/" + k.Environment + "/" + k.Name + "/" + k.Version + "/" + k.Region + "/" + k.Hostname + "/" + k.ID
}

// RegisterResult is a response to Register request from Kite to Kontrol.
type RegisterResult struct {
	// IP address seen by kontrol
	PublicIP string
}

type GetKitesResult struct {
	Kites     []*KiteWithToken `json:"kites"`
	WatcherID string           `json:"watcherID"`
}

type KiteWithToken struct {
	Kite  Kite   `json:"kite"`
	Token string `json:"token"`
}

// KiteEvent is the struct that is sent as an argument in watchCallback of
// getKites method of Kontrol.
type KiteEvent struct {
	Action KiteAction `json:"action"`
	Kite   Kite       `json:"kite"`

	// Required when Action == Register
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
