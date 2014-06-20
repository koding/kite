package sockjsclient

// http://sockjs.github.io/sockjs-protocol/sockjs-protocol-0.3.3.html

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func ConnectWebsocketSession(baseURL string) (*WebsocketSession, error) {
	dialURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	// will be used to set the origin header
	originalScheme := dialURL.Scheme

	if err := replaceSchemeWithWS(dialURL); err != nil {
		return nil, err
	}

	if err := addMissingPortAndSlash(dialURL); err != nil {
		return nil, err
	}

	serverID := threeDigits()
	sessionID := randomStringLength(20)

	// Add server_id and session_id to the path.
	dialURL.Path += serverID + "/" + sessionID + "/websocket"

	requestHeader := http.Header{}
	requestHeader.Add("Origin", originalScheme+"://"+dialURL.Host)

	conn, _, err := websocket.DefaultDialer.Dial(dialURL.String(), requestHeader)
	if err != nil {
		return nil, err
	}

	session := NewWebsocketSession(conn)
	session.id = sessionID
	return session, nil
}

type WebsocketSession struct {
	conn     *websocket.Conn
	id       string
	messages []string
}

func NewWebsocketSession(conn *websocket.Conn) *WebsocketSession {
	return &WebsocketSession{
		conn: conn,
	}
}

func (w *WebsocketSession) RemoteAddr() string {
	return w.conn.RemoteAddr().String()
}

// ID returns a session id
func (w *WebsocketSession) ID() string {
	return w.id
}

// Recv reads one text frame from session
func (w *WebsocketSession) Recv() (string, error) {
	// Return previously received messages if there is any.
	if len(w.messages) > 0 {
		msg := w.messages[0]
		w.messages = w.messages[1:]
		return msg, nil
	}

read_frame:
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
		// TODO handle open
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
		return "", errors.New("session closed")
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
	b, err := json.Marshal([]string{str})
	if err != nil {
		return err
	}

	return w.conn.WriteMessage(websocket.TextMessage, b)
}

// Close closes the session with provided code and reason.
func (w *WebsocketSession) Close(status uint32, reason string) error {
	return w.conn.Close()
}

var r = rand.New(rand.NewSource(time.Now().UnixNano()))

// threeDigits is used to generate a server_id.
func threeDigits() string {
	var i uint64
	i = uint64(r.Int31())
	if i < 100 {
		i += 100
	}
	return strconv.FormatUint(i, 10)[:3]
}

// randomStringLength is used to generate a session_id.
func randomStringLength(length int) string {
	size := (length * 6 / 8) + 1
	r := make([]byte, size)
	crand.Read(r)
	return base64.URLEncoding.EncodeToString(r)[:length]
}

func replaceSchemeWithWS(u *url.URL) error {
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("invalid scheme in url: %s", u.Scheme)
	}
	return nil
}

// addMissingPortAndSlash appends 80 or 443 depending on the scheme
// if there is no port number in the URL.
// Also it adds "/" to the end of path if path does not ends with "/".
func addMissingPortAndSlash(u *url.URL) error {
	_, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		if missingPortErr, ok := err.(*net.AddrError); ok && missingPortErr.Err == "missing port in address" {
			var port string
			switch u.Scheme {
			case "ws":
				port = "80"
			case "wss":
				port = "443"
			default:
				return fmt.Errorf("unknown scheme: %s", u.Scheme)
			}
			u.Host = net.JoinHostPort(strings.TrimRight(missingPortErr.Addr, ":"), port)
		} else {
			return err
		}
	}

	if u.Path == "" || u.Path[len(u.Path)-1:] != "/" {
		u.Path += "/"
	}

	return nil
}
