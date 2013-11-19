package rpc

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"koding/newkite/dnode"
)

// Server is a websocket server serving each dnode messages with registered handlers.
type Server struct {
	websocket.Server

	// Base Dnode instance that holds registered methods.
	// It is copied for each connection with Dnode.Copy().
	dnode *dnode.Dnode

	// Called when a client is connected
	onConnectHandlers []func(*Client)

	// Called when a client is disconnected
	onDisconnectHandlers []func(*Client)
}

func NewServer() *Server {
	s := &Server{dnode: dnode.New(nil)}
	// Need to set this because websocket.Server is embedded.
	s.Handler = s.handleWS
	return s
}

func (s *Server) Handle(method string, handler dnode.Handler) {
	s.dnode.Handle(method, handler)
}

func (s *Server) HandleFunc(method string, handler func(*dnode.Message, dnode.Transport)) {
	s.dnode.HandleFunc(method, handler)
}

func (s *Server) HandleSimple(method string, handler interface{}) {
	s.dnode.HandleSimple(method, handler)
}

// handleWS is the websocket connection handler.
func (s *Server) handleWS(ws *websocket.Conn) {
	defer ws.Close()

	fmt.Println("--- connected new client")

	// This client is actually is the server for the websocket.
	// Since both sides can send/receive messages the client code is reused here.
	clientServer := NewClient()
	clientServer.Conn = ws
	clientServer.Dnode = s.dnode.Copy(clientServer)

	s.callOnConnectHandlers(clientServer)

	// Run after methods are registered and delegate is set
	clientServer.run()

	s.callOnDisconnectHandlers(clientServer)
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
