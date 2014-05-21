package kite

import (
	"encoding/json"
	"errors"

	"code.google.com/p/go.net/websocket"
)

type WebsocketSession struct {
	conn     *websocket.Conn
	messages []string
}

func NewWebsocketSession(conn *websocket.Conn) *WebsocketSession {
	return &WebsocketSession{
		conn: conn,
	}
}

// ID returns a session id
func (session *WebsocketSession) ID() string {
	// TODO Return real ID
	return "<id>"
}

// Recv reads one text frame from session
func (session *WebsocketSession) Recv() (string, error) {
	// Return previously received messages if there is any.
	if len(session.messages) > 0 {
		msg := session.messages[0]
		session.messages = session.messages[1:]
		return msg, nil
	}

read_frame:
	// Read one SockJS frame.
	var frame string
	err := websocket.Message.Receive(session.conn, &frame)
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
		session.messages = append(session.messages, messages...)
	case 'm':
		var message string
		err = json.Unmarshal(data, &message)
		if err != nil {
			return "", err
		}
		session.messages = append(session.messages, message)
	case 'c':
		return "", errors.New("session closed")
	case 'h':
		// TODO handle heartbeat
		goto read_frame
	default:
		return "", errors.New("invalid frame type")
	}

	// Return first message in slice.
	if len(session.messages) == 0 {
		return "", errors.New("no message")
	}
	msg := session.messages[0]
	session.messages = session.messages[1:]
	return msg, nil
}

// Send sends one text frame to session
func (session *WebsocketSession) Send(s string) error {
	b, _ := json.Marshal([]string{s})
	return websocket.Message.Send(session.conn, b)
}

// Close closes the session with provided code and reason.
func (session *WebsocketSession) Close(status uint32, reason string) error {
	return session.conn.Close()
}
