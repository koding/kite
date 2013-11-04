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

// Kite is the base struct containing the public fields.
// It is usually embeded in other structs, including the db model.
type Kite struct {
	// Short name identifying the type of the kite. Example: fs, terminal...
	Name string `bson:"name" json:"name"`

	// Owner of the Kite
	Username string `bson:"username" json:"username"`

	// Every Kite instance has different identifier.
	// If a kite is restarted, it's id will change.
	// This is generated on the Kite.
	ID string `bson:"_id" json:"id"`

	// This is used temporary to distinguish kites that are used for Koding
	// client-side. An example is to use it with value "vm"
	Kind string `bson:"kind" json:"kind"`

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

// BrowserToKontrolRequest is a structure of message sent
// from Browser to Kontrol.
type BrowserToKontrolRequest struct {
	Username  string `json:"username"`
	Kitename  string `json:"kitename"`
	SessionID string `json:"sessionID"`
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
	Port         string `json:"port"`
	Version      string `json:"version"`
	Kind         string `json:"kind"`
	KontrolAddr  string `json:"kontrolAddr"`
	Dependencies string `json:"dependencies"`
}
