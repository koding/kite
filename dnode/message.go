// Package dnode implements a dnode scrubber.
// See the following URL for details:
// https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown
package dnode

// Message is the JSON object to call a method at the other side.
type Message struct {
	// Method can be an integer or string.
	Method interface{} `json:"method"`

	// Array of arguments
	Arguments *Partial `json:"arguments"`

	// Integer map of callback paths in arguments
	Callbacks map[string]Path `json:"callbacks"`
}
