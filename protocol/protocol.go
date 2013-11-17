// Package protocol defines the communication between the components
// of the Kite infrastructure. It defines some constants and structures
// designed to be sent between those components.
//
// The following table shows the communication types:
//
// +-----------------+---------+----------+----------------+
// |                 | Library | Protocol | Authentication |
// +-----------------+---------+----------+----------------+
// | Browser-Kontrol | moh     | JSON     | SessionID      |
// | Kite-Kontrol    | moh     | JSON     | Koding Key     |
// | Browser-Kite    | Go-RPC  | dnode    | token          |
// | Kite-Kite       | Go-RPC  | gob      | token          |
// +-----------------+---------+----------+----------------+
//
package protocol

import (
	"koding/tools/dnode"
	"net"
	"time"
)

const HEARTBEAT_INTERVAL = time.Millisecond * 1000
const HEARTBEAT_DELAY = time.Millisecond * 1000

// Kite's HTTP server runs a RPC server here
const WEBSOCKET_PATH = "/sock"

// Needed by kontrol to filter messages from other connections.
const KitesSubscribePrefix = "kites"

// Kite is the base struct containing the public fields. It is usually embeded
// in other structs, including the db model. The access model is in the form:
// username.environment.name.version.region.hostname.id
type Kite struct {
	// Short name identifying the type of the kite. Example: fs, terminal...
	Name string `bson:"name" json:"name"`

	// Owner of the Kite
	Username string `bson:"username" json:"username"`

	// Every Kite instance has different identifier.
	// If a kite is restarted, it's id will change.
	// This is generated on the Kite.
	ID string `bson:"_id" json:"id"`

	// Environment is defines as something like "production", "testing",
	// "staging" or whatever.  This allows you to differentiate between a
	// cluster of kites.
	Environment string `bson:"environment" json:"environment"`

	// Region of the kite it is running. Like "Europe", "Asia" or some other
	// locations.
	Region string `bson:"region" json:"region"`

	Version  string `bson:"version" json:"version"`
	Hostname string `bson:"hostname" json:"hostname"`
	PublicIP string `bson:"publicIP" json:"publicIP"`
	Port     string `bson:"port" json:"port"`
}

func (k *Kite) Addr() string {
	return net.JoinHostPort(k.PublicIP, k.Port)
}

// KiteRequest is a structure that is used in Kite-to-Kite communication.
type KiteRequest struct {
	Kite   `json:"kite"`
	Token  string      `json:"token"`
	Method string      `json:"method"`
	Args   interface{} `json:"args"`
}

// KiteDnodeRequest is the data structure sent when a request is made
// from a client to the RPC server of a Kite.
type KiteDnodeRequest struct {
	Kite
	Method string
	Args   *dnode.Partial // Must include a token for authentication
}

// KiteToKontrolRequest is a structure of message sent
// from Kites and Kontrol.
type KiteToKontrolRequest struct {
	Kite      Kite                   `json:"kite"`
	KodingKey string                 `json:"kodingKey"`
	Method    Method                 `json:"method"`
	Args      map[string]interface{} `json:"args"`
}

type Method string

const (
	Pong         Method = "PONG"
	RegisterKite Method = "REGISTER_KITE"
	GetKites     Method = "GET_KITES"
)

// RegisterResponse is a response to Register request from Kite to Kontrol.
type RegisterResponse struct {
	Result RegisterResult `json:"result"`

	// Username is sent in response because the kite does not know
	// it's own user's name on start.
	Username string `json:"username"`

	// PublicIP is the IP address visible to Kontrol.
	PublicIP string
}

type RegisterResult string

const (
	AllowKite  RegisterResult = "ALLOW"
	RejectKite RegisterResult = "REJECT"
)

type GetKitesResponse []KiteWithToken

type KiteWithToken struct {
	Kite  `json:"kite"`
	Token string `json:"token"`
}

// KontrolMessage is a structure that is published from Kontrol to Kite
// to notify some events.
type KontrolMessage struct {
	Type MessageType            `json:"type"`
	Args map[string]interface{} `json:"args"`
}

type MessageType string

const (
	KiteRegistered   MessageType = "KITE_REGISTERED"
	KiteDisconnected MessageType = "KITE_DISCONNECTED"
	KiteUpdated      MessageType = "KITE_UPDATED"
	Ping             MessageType = "PING"
)

type Options struct {
	Username     string `json:"username"`
	Kitename     string `json:"kitename"`
	LocalIP      string `json:"localIP"`
	PublicIP     string `json:"publicIP"`
	Environment  string `json:"environment"`
	Region       string `json:"region"`
	Port         string `json:"port"`
	Version      string `json:"version"`
	KontrolAddr  string `json:"kontrolAddr"`
	Dependencies string `json:"dependencies"`
}

// KontrolQuery is a structure of message sent to Kontrol. It is used for
// querying kites based on the incoming field parameters. Missing fields are
// not counted during the query (for example if the "version" field is empty,
// any kite with different version is going to be matched).
type KontrolQuery struct {
	Username    string `json:"username"`
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Region      string `json:"region"`
	Hostname    string `json:"hostname"`
	ID          string `json:"id"`

	// Authentication is used to autenticate the client who made the
	// KontrolQuery. The authentication process is based on the type.
	// Currently there are two types of authentication flows. One is "browser"
	// other one is "kite". If "browser" is set the key should be the session
	// id of the koding user. If "kite" is set the key should be the content
	// of the kite's kodingkey.
	Authentication struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	} `json:"authentication"`
}
