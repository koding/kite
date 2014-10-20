package kontrol

import (
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

// Storage is an interface to a kite storage. A storage should be safe to
// concurrent access.
type Storage interface {
	// Get retrieves the Kites with the given query
	Get(query *protocol.KontrolQuery) (Kites, error)

	// Add inserts the given kite with the given value
	Add(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error

	// Update updates the value for the given kite
	Update(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error

	// Delete deletes the given kite from the storage
	Delete(kite *protocol.Kite) error

	// Upsert inserts or updates the value for the given kite
	Upsert(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error
}
