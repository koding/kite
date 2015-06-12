package protocol

// RegisterValue is the type of the value that is saved to the storage
type RegisterValue struct {
	// URL is the Kite's URL that can be accessed
	URL string `json:"url"`

	// KeyId specifies the public-private key pair reference the kite is using.
	// This is currently only used by Kontrol itself internally, however it
	// might be changed in the future.
	KeyID string `json:"key_id"`
}
