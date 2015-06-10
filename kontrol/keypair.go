package kontrol

// KeyPair defines a single key pair entity
type KeyPair struct {
	// ID is the unique id defining the key pair
	ID string

	// Public key
	Public string

	// Private key
	Private string
}

// KeyPairStorage is responsible of managing key pairs
type KeyPairStorage interface {
	// AddKey adds the given key pair to the storage
	AddKey(*KeyPair) error

	// DeleteKey deletes the given key pairs from the storage
	DeleteKey(*KeyPair) error

	// GetKeyFromID retrieves the KeyPair from the given ID
	GetKeyFromID(id string) (*KeyPair, error)

	// GetKeyFromPublic retrieves the KeyPairs from the given public Key
	GetKeyFromPublic(publicKey string) (*KeyPair, error)
}
