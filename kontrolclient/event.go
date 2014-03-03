package kontrolclient

import (
	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

// Event is the struct that is emitted from Kontrol.WatchKites method.
type Event struct {
	protocol.KiteEvent

	localKite *kite.Kite
}

// Create new Client from Register events. It panics if the action is not
// Register.
func (e *Event) Client() *kite.Client {
	if e.Action != protocol.Register {
		panic("This method can only be called for 'Register' event.")
	}

	r := e.localKite.NewClientString(e.URL)
	r.Kite = e.Kite
	r.Authentication = &kite.Authentication{
		Type: "token",
		Key:  e.Token,
	}
	return r
}
