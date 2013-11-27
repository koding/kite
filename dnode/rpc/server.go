package rpc

import (
	"code.google.com/p/go.net/websocket"
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

// Handle registers the handler for the given method.
// If a handler already exists for method, Handle panics.
func (s *Server) Handle(method string, handler dnode.Handler) {
	s.dnode.Handle(method, handler)
}

// HandleFunc registers the handler function for the given method.
func (s *Server) HandleFunc(method string, handler func(*dnode.Message, dnode.Transport)) {
	s.dnode.HandleFunc(method, handler)
}

// HandleSimple registers the handler function for given method.
// The difference from HandleFunc() that all dnode message arguments are passed
// directly to the handler instead of Message and Transport.
func (s *Server) HandleSimple(method string, handler interface{}) {
	s.dnode.HandleSimple(method, handler)
}

// handleWS is the websocket connection handler.
func (s *Server) handleWS(ws *websocket.Conn) {
	defer ws.Close()

	// This client is actually is the server for the websocket.
	// Since both sides can send/receive messages the client code is reused here.
	clientServer := s.NewClientWithHandlers()
	clientServer.Conn = ws

	s.callOnConnectHandlers(clientServer)

	// Run after methods are registered and delegate is set
	clientServer.run()

	s.callOnDisconnectHandlers(clientServer)
}

// NewClientWithHandlers returns a pointer to new Client.
// The returned Client will have the same handlers with the server.
func (s *Server) NewClientWithHandlers() *Client {
	c := NewClient()
	c.dnode = s.dnode.Copy(c)
	return c
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
}

func (s *Server) callOnDisconnectHandlers(c *Client) {
	for _, handler := range s.onDisconnectHandlers {
		go handler(c)
	}
}
