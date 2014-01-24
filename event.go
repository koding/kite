package kite

import (
	"koding/newkite/protocol"
)

// Event is the struct that is sent as an argument in watchCallback of
// getKites method of Kontrol.
type Event struct {
	protocol.KiteEvent

	localKite *Kite
}

func (e *Event) RemoteKite() *RemoteKite {
	if e.Action != protocol.Register {
		panic("This method can only be called for 'Register' event.")
	}

	auth := Authentication{
		Type: "token",
		Key:  e.Token,
	}

	return e.localKite.NewRemoteKite(e.Kite, auth)
}
