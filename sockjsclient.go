package kite

// http://sockjs.github.io/sockjs-protocol/sockjs-protocol-0.3.3.html

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.net/websocket"
)

func ConnectWebsocketSession(baseURL string) (*WebsocketSession, error) {
	config, err := websocket.NewConfig(baseURL, baseURL)
	if err != nil {
		return nil, err
	}
	config.Origin.Path = ""

	if err = replaceSchemeWithWS(config.Location); err != nil {
		return nil, err
	}

	if err = addMissingPortAndSlash(config.Location); err != nil {
		return nil, err
	}

	id := threeDigits()

	// Add server_id and session_id to the path.
	config.Location.Path += id + "/" + randomStringLength(20) + "/websocket"

	conn, err := websocket.DialConfig(config)
	if err != nil {
		return nil, err
	}

	session := NewWebsocketSession(conn)
	session.id = id
	return session, nil
}

type WebsocketSession struct {
	conn     *websocket.Conn
	config   *websocket.Config
	id       string
	messages []string
}

func NewWebsocketSession(conn *websocket.Conn) *WebsocketSession {
	return &WebsocketSession{
		conn: conn,
	}
}

// ID returns a session id
func (s *WebsocketSession) ID() string {
	return s.id
}

// Recv reads one text frame from session
func (s *WebsocketSession) Recv() (string, error) {
	// Return previously received messages if there is any.
	if len(s.messages) > 0 {
		msg := s.messages[0]
		s.messages = s.messages[1:]
		return msg, nil
	}

read_frame:
	// Read one SockJS frame.
	var frame string
	err := websocket.Message.Receive(s.conn, &frame)
	if err != nil {
		return "", err
	}
	if len(frame) == 0 {
		return "", errors.New("unexpected empty message")
	}

	frameType := frame[0]
	data := []byte(frame[1:])

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
		s.messages = append(s.messages, messages...)
	case 'm':
		var message string
		err = json.Unmarshal(data, &message)
		if err != nil {
			return "", err
		}
		s.messages = append(s.messages, message)
	case 'c':
		return "", errors.New("session closed")
	case 'h':
		// TODO handle heartbeat
		goto read_frame
	default:
		return "", errors.New("invalid frame type")
	}

	// Return first message in slice.
	if len(s.messages) == 0 {
		return "", errors.New("no message")
	}
	msg := s.messages[0]
	s.messages = s.messages[1:]
	return msg, nil
}

// Send sends one text frame to session
func (s *WebsocketSession) Send(str string) error {
	b, _ := json.Marshal([]string{str})
	return websocket.Message.Send(s.conn, b)
}

// Close closes the session with provided code and reason.
func (s *WebsocketSession) Close(status uint32, reason string) error {
	return s.conn.Close()
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
