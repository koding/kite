package config

// Transport defines the underlying transport to be used
type Transport int

const (
	WebSocket = iota
	XHRPolling
)

func (t Transport) String() string {
	switch t {
	case WebSocket:
		return "WebSocket"
	case XHRPolling:
		return "XHRPolling"
	default:
		return "UnkownKiteTransport"
	}
}

var Transports = map[string]Transport{
	"WebSocket":  WebSocket,
	"XHRPolling": XHRPolling,
}
