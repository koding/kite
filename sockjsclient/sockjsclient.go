package sockjsclient

// http://sockjs.github.io/sockjs-protocol/sockjs-protocol-0.3.3.html

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koding/kite/config"
	"github.com/koding/kite/utils"

	"github.com/igm/sockjs-go/sockjs"
)

// ErrSessionClosed is returned by Send/Recv methods when
// calling them after the session got closed.
//
// Deprecated: Send/Recv methods return *ErrSession error
// with State set to sockjs.SessionClosed instead.
var ErrSessionClosed = &ErrSession{
	State: sockjs.SessionClosed,
}

// ErrSession is returned by Send/Recv methods when
// the underlying session state change is responsible
// for the failure.
type ErrSession struct {
	Type  config.Transport
	State sockjs.SessionState // session state
	Err   error               // more detailed description of the problem
}

var stateTexts = map[sockjs.SessionState]string{
	sockjs.SessionActive:  "session is active",
	sockjs.SessionOpening: "session is opening",
	sockjs.SessionClosing: "session is closing",
	sockjs.SessionClosed:  "session is closed",
}

// Error implements the buildin error interface.
func (err *ErrSession) Error() string {
	if err.Err == nil {
		return fmt.Sprintf("%s (%s)", stateTexts[err.State], err.Type)
	}
	return fmt.Sprintf("%s: %s (%s)", stateTexts[err.State], err.Err, err.Type)
}

// IsSessionClosed tests whether given error is caused
// by a closed session.
func IsSessionClosed(err error) bool {
	switch err {
	case ErrSessionClosed, sockjs.ErrSessionNotOpen, websocket.ErrCloseSent:
		return true
	}
	if e, ok := err.(*ErrSession); ok && e.State > sockjs.SessionActive {
		return true
	}
	_, ok := err.(*websocket.CloseError)
	return ok
}

// WebsocketSession represents a sockjs.Session over
// a websocket connection.
type WebsocketSession struct {
	id       string
	messages []string
	closed   int32
	req      *http.Request

	mu    sync.Mutex
	conn  *websocket.Conn
	state sockjs.SessionState
}

var _ sockjs.Session = (*WebsocketSession)(nil)

// DialOptions are used to overwrite default behavior
// of the websocket session.
//
// Deprecated: Use *config.Config struct instead for
// configuring SockJS connection.
type DialOptions struct {
	// URL of the remote kite.
	//
	// Required.
	BaseURL string

	// ReadBufferSize is the buffer size used for
	// reads on a websocket connection.
	//
	// Deprecated: Set Config.Dialer.ReadBufferSize of
	// the local kite instead.
	ReadBufferSize int

	// WriteBufferSize is the buffer size used for
	// writes on a websocket connection.
	//
	// Deprecated: Set Config.Dialer.WriteBufferSize of the
	// the local kite instead.
	WriteBufferSize int

	// Timeout specifies dial timeout
	//
	// Deprecated: Set Config.Dialer.Dial of the local kite instead.
	Timeout time.Duration

	// ClientFunc gives new HTTP client for use with XHR connections.
	//
	// Deprecated: Set Config.ClientFunc of the local kite instead.
	ClientFunc func(*DialOptions) *http.Client
}

func (opts *DialOptions) client() *http.Client {
	if opts.ClientFunc != nil {
		return opts.ClientFunc(opts)
	}
	return &http.Client{
		Timeout: opts.Timeout,
		Jar:     config.CookieJar,
	}
}

// DialWebsocket establishes a SockJS session over a websocket connection.
//
// Requires cfg.Websocket to be a valid client.
func DialWebsocket(uri string, cfg *config.Config) (*WebsocketSession, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	h := http.Header{
		"Origin": {u.Scheme + "://" + u.Host},
	}

	serverID := threeDigits()
	sessionID := utils.RandomString(20)

	u = makeWebsocketURL(u, serverID, sessionID)

	conn, _, err := cfg.Websocket.Dial(u.String(), h)
	if err != nil {
		return nil, err
	}

	session := NewWebsocketSession(conn)
	session.id = sessionID
	session.req = &http.Request{
		URL:    u,
		Header: h,
	}

	return session, nil
}

// ConnectWebsocketSession dials the remote specified in the opts and
// creates new websocket session.
//
// Deprecated: Use DialWebsocket instead.
func ConnectWebsocketSession(opts *DialOptions) (*WebsocketSession, error) {
	cfg := config.New()
	cfg.Websocket.HandshakeTimeout = opts.Timeout
	cfg.Websocket.ReadBufferSize = opts.ReadBufferSize
	cfg.Websocket.WriteBufferSize = opts.WriteBufferSize

	return DialWebsocket(opts.BaseURL, cfg)
}

// NewWebsocketSession creates new sockjs.Session from existing
// websocket connection.
func NewWebsocketSession(conn *websocket.Conn) *WebsocketSession {
	return &WebsocketSession{
		conn: conn,
	}
}

// RemoteAddr gives network address of the remote client.
func (w *WebsocketSession) RemoteAddr() string {
	return w.conn.RemoteAddr().String()
}

// ID returns a session id.
func (w *WebsocketSession) ID() string {
	return w.id
}

// Recv reads one text frame from session.
func (w *WebsocketSession) Recv() (string, error) {
	// Return previously received messages if there is any.
	if len(w.messages) > 0 {
		msg := w.messages[0]
		w.messages = w.messages[1:]
		return msg, nil
	}

read_frame:
	if atomic.LoadInt32(&w.closed) == 1 {
		return "", ErrSessionClosed
	}

	// Read one SockJS frame.
	_, buf, err := w.conn.ReadMessage()
	if err != nil {
		return "", err
	}

	if len(buf) == 0 {
		return "", errors.New("unexpected empty message")
	}

	frameType := buf[0]
	data := buf[1:]

	switch frameType {
	case 'o':
		w.setState(sockjs.SessionActive)
		goto read_frame
	case 'a':
		var messages []string
		err = json.Unmarshal(data, &messages)
		if err != nil {
			return "", err
		}
		w.messages = append(w.messages, messages...)
	case 'm':
		var message string
		err = json.Unmarshal(data, &message)
		if err != nil {
			return "", err
		}
		w.messages = append(w.messages, message)
	case 'c':
		w.setState(sockjs.SessionClosed)
		return "", ErrSessionClosed
	case 'h':
		// TODO handle heartbeat
		goto read_frame
	default:
		return "", errors.New("invalid frame type")
	}

	// Return first message in slice.
	if len(w.messages) == 0 {
		return "", errors.New("no message")
	}
	msg := w.messages[0]
	w.messages = w.messages[1:]
	return msg, nil
}

// Send sends one text frame to session
func (w *WebsocketSession) Send(str string) error {
	if atomic.LoadInt32(&w.closed) == 1 {
		return ErrSessionClosed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	b, _ := json.Marshal([]string{str})
	return w.conn.WriteMessage(websocket.TextMessage, b)
}

// Close closes the session with provided code and reason.
func (w *WebsocketSession) Close(uint32, string) error {
	if atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return w.conn.Close()
	}

	return ErrSessionClosed
}

func (w *WebsocketSession) setState(state sockjs.SessionState) {
	w.mu.Lock()
	w.state = state
	w.mu.Unlock()
}

// GetSessionState gives state of the session.
func (w *WebsocketSession) GetSessionState() sockjs.SessionState {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.state
}

// Request implements the sockjs.Session interface.
func (w *WebsocketSession) Request() *http.Request {
	return w.req
}

// threeDigits is used to generate a server_id.
func threeDigits() string {
	return strconv.FormatInt(100+int64(utils.Int31n(900)), 10)
}

func makeWebsocketURL(u *url.URL, serverID, sessionID string) *url.URL {
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}

	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		if u.Scheme == "wss" {
			u.Host = net.JoinHostPort(u.Host, "443")
		} else {
			u.Host = net.JoinHostPort(u.Host, "80")
		}
	}

	u.Path = path.Join(u.Path, serverID, sessionID, "websocket")

	return u
}
