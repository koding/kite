package kontrol

import (
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

// Storage is an interface to a kite storage.
type Storage interface {
	// Get retrieves the Kites with the given query
	Get(query *protocol.KontrolQuery) (Kites, error)

	// Set stores the given kite with the given value
	Set(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error

	// Delete deletes the given kite from the storage
	Delete(kite *protocol.Kite) error
}
