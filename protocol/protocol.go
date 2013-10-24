package protocol

import (
	"koding/db/models"
	"koding/tools/dnode"
	"time"
)

const HEARTBEAT_INTERVAL = time.Millisecond * 500
const HEARTBEAT_DELAY = time.Millisecond * 500

const WEBSOCKET_PATH = "/sock"

const ORIGIN_JSON = "json"
const ORIGIN_GOB = "gob"

const (
	AllowKite  = "AllowKite"
	PermitKite = "PermitKite"
	AddKite    = "AddKite"
	RemoveKite = "RemoveKite"
	UpdateKite = "UpdateKite"
)

type KiteRequest struct {
	models.KiteBase
	Method string
	Origin string
	Args   interface{}
}

type KiteDnodeRequest struct {
	models.KiteBase
	Method string
	Origin string
	Args   *dnode.Partial
}

type Request struct {
	models.KiteBase
	RemoteKite string `json:"remoteKite"`
	SessionID  string `json:"sessionID"`
	Action     string
}

type RegisterResponse struct {
	Addr     string            `json:"addr"`
	Result   string            `json:"result"`
	Username string            `json:"username"`
	Token    *models.KiteToken `json:"token"`
}

type PubResponse struct {
	models.KiteBase
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
	KontrolAddr  string `json:"kontrolAddr"`
	Dependencies string `json:"dependencies"`
}
