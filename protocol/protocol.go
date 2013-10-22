package protocol

import (
	"koding/tools/dnode"
	"labix.org/v2/mgo/bson"
	"time"
)

const HEARTBEAT_INTERVAL = time.Millisecond * 500
const HEARTBEAT_DELAY = time.Millisecond * 500

const FRAME_SEPARATOR = ":"
const WEBSOCKET_PATH = "/sock"

const ORIGIN_JSON = "json"
const ORIGIN_GOB = "gob"

const DEBUG_ENABLED = false

const (
	AllowKite  = "AllowKite"
	PermitKite = "PermitKite"
	AddKite    = "AddKite"
	RemoveKite = "RemoveKite"
	UpdateKite = "UpdateKite"
)

type Base struct {
	Id        bson.ObjectId `bson:"_id" json:"-"`
	Username  string        `bson:"username" json:"username"`
	Kitename  string        `bson:"kitename" json:"kitename"`
	Version   string        `bson:"version" json:"version"`
	PublicKey string        `bson:"publicKey" json:"publicKey"`
	Token     string        `bson:"token" json:"token"`
	Uuid      string        `bson:"uuid" json:"uuid"`
	Hostname  string        `bson:"hostname" json:"hostname"`
	Addr      string        `bson:"addr" json:"addr"`

	// this is used temporary to distinguish kites that are used for Koding
	// client-side. An example is to use it with value "vm"
	Kind string `bson:"kind" json:"kind"`

	LocalIP  string `bson:"localIP" json:"localIP"`
	PublicIP string `bson:"publicIP" json:"publicIP"`
	Port     string `bson:"port" json:"port"`
}

type KiteRequest struct {
	Base
	Method string
	Origin string
	Args   interface{}
}

type KiteDnodeRequest struct {
	Base
	Method string
	Origin string
	Args   *dnode.Partial
}

type Request struct {
	Base
	RemoteKite string `json:"remoteKite"`
	SessionID  string `json:"sessionID"`
	Action     string
}

type RegisterResponse struct {
	Addr     string `json:"addr"`
	Result   string `json:"result"`
	Username string `json:"username"`
	Token    Token  `json:"token"`
}

type PubResponse struct {
	Base
	Action string `json:"action"`
}

type Options struct {
	Username     string `json:"username"`
	Kitename     string `json:"kitename"`
	LocalIP      string `json:"localIP"`
	PublicIP     string `json:"publicIP"`
	Port         string `json:"port"`
	Version      string `json:"version"`
	Kind         string `json:"kind"`
	Dependencies string `json:"dependencies"`
}

type Token struct {
	ID        string        `json:"id"`
	Username  string        `json:"username"`
	Expire    time.Duration `json:"expire"`
	CreatedAt time.Time     `json:"createdAt"`
}
