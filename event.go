package kite

import (
	"github.com/koding/kite/protocol"
)

// Event is the struct that is emitted from Kontrol.WatchKites method.
type Event struct {
	protocol.KiteEvent

	localKite *Kite
}

// Create new RemoteKite from Register events. It panics if the action is not
// Register.
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
