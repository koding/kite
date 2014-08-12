package protocol

// RegisterValue is the type of the value that is saved to etcd.
type RegisterValue struct {
	URL string `json:"url"`
}
