package protocol

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	errInvalidChar = errors.New("message contains invalid chars")
	errInvalidOp   = errors.New("invalid start operation")
)

// WebRTCSignalMessage represents a signalling message between peers and the singalling server
type WebRTCSignalMessage struct {
	Type    string          `json:"type,omitempty"`
	Src     string          `json:"src,omitempty"`
	Dst     string          `json:"dst,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`

	parsedPayload *Payload
	isParsed      bool
	mu            sync.Mutex
}

// Payload is the content of `payload` in the json
type Payload struct {
	Msg *string `json:"msg,omitempty"`
	Sdp *struct {
		Type *string `json:"type,omitempty"`
		Sdp  *string `json:"sdp,omitempty"`
	} `json:"sdp,omitempty"`
	Type          *string `json:"type,omitempty"`
	Label         *string `json:"label,omitempty"`
	ConnectionID  *string `json:"connectionId,omitempty"`
	Reliable      *bool   `json:"reliable,omitempty"`
	Serialization *string `json:"serialization,omitempty"`
	Browser       *string `json:"browser,omitempty"`
	Candidate     *struct {
		Candidate     *string `json:"candidate,omitempty"`
		SdpMid        *string `json:"sdpMid,omitempty"`
		SdpMLineIndex *int    `json:"sdpMLineIndex,omitempty"`
	} `json:"candidate,omitempty"`
}

// ParsePayload parses the payload if it is not parsed previously. This method
// can be called concurrently.
func (w *WebRTCSignalMessage) ParsePayload() (*Payload, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isParsed {
		return w.parsedPayload, nil
	}

	payload := &Payload{}
	if err := json.Unmarshal(w.Payload, payload); err != nil {
		return nil, err
	}

	w.parsedPayload = payload
	return payload, nil
}

// ParseWebRTCSignalMessage parses the web rtc command/message
func ParseWebRTCSignalMessage(msg string) (*WebRTCSignalMessage, error) {
	// All messages are text (utf-8 encoded at present)
	if !utf8.Valid([]byte(msg)) {
		return nil, errInvalidChar
	}

	w := &WebRTCSignalMessage{}
	if err := json.Unmarshal([]byte(msg), w); err != nil {
		return nil, err
	}

	if err := validateOperation(w.Type); err != nil {
		return nil, err
	}

	return w, nil
}

func validateOperation(op string) error {
	switch strings.ToUpper(op) {
	case "ANSWER", "OFFER", "CANDIDATE", "LEAVE":
		return nil
	default:
		return errInvalidOp
	}
}
