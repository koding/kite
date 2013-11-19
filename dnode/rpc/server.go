package rpc

import (
	"code.google.com/p/go.net/websocket"
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"reflect"
)

// Server is a websocket server serving each dnode messages with registered handlers.
type Server struct {
	websocket.Server

	// Functions registered with HandleFunc() are saved here
	handlers map[string]interface{}

	// Unknown methods are precessed by this handler
	Delegate dnode.MessageHandler

	// Called when a client is connected
	onConnectHandlers []func(*Client)

	// Called when a client is disconnected
	onDisconnectHandlers []func(*Client)
}

func NewServer() *Server {
	s := &Server{
		handlers: make(map[string]interface{}),
	}
	s.Handler = s.handleWS
	return s
}

// HandleFunc registers a function to run on "method".
func (s *Server) HandleFunc(method string, handler interface{}) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		panic(errors.New("handler is not a func"))
	}

	s.handlers[method] = handler
}

// handleWS is the websocket connection handler.
func (s *Server) handleWS(ws *websocket.Conn) {
	defer ws.Close()

	fmt.Println("--- connected new client")
	// This client is actually is the server for the websocket.
	// Since both sides can send/receive messages the client code is reused here.
	client := NewClient()
	client.Conn = ws

	// Pass dnode message delegate
	client.Dnode.ExternalHandler = s.Delegate

	// Add our servers handler methods to the client.
	for method, handler := range s.handlers {
		client.Dnode.HandleFunc(method, handler)
	}

	s.callOnConnectHandlers(client)

	// Run after methods are registered and delegate is set
	client.run()

	s.callOnDisconnectHandlers(client)
}

func (s *Server) OnConnect(handler func(*Client)) {
	s.onConnectHandlers = append(s.onConnectHandlers, handler)
}

func (s *Server) OnDisconnect(handler func(*Client)) {
	s.onDisconnectHandlers = append(s.onDisconnectHandlers, handler)
}

func (s *Server) callOnConnectHandlers(c *Client) {
	for _, handler := range s.onConnectHandlers {
		go handler(c)
	}
	// It is unnecessary to call c.connected() here because the client is
	// already created by this server and we did not attach any handlers to
	// run on connect.
}

func (s *Server) callOnDisconnectHandlers(c *Client) {
	// We are also triggering the disconnect event on the client because
	// there may be a handler registered on it.
	c.callOnDisconnectHandlers()

	for _, handler := range s.onDisconnectHandlers {
		go handler(c)
	}
}
