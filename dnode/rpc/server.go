// Package rpc implement dnode rpc client and server.
package rpc

import (
	"code.google.com/p/go.net/websocket"
	"github.com/koding/kite/dnode"
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

	// A space for saving/reading extra properties to be passed to all clients.
	properties map[string]interface{}
}

func NewServer() *Server {
	s := &Server{
		dnode:      dnode.New(nil),
		properties: make(map[string]interface{}),
	}
	// Need to set this because websocket.Server is embedded.
	s.Handler = s.handleWS
	return s
}

func (s *Server) SetConcurrent(value bool) {
	s.dnode.SetConcurrent(value)
}

func (s *Server) SetWrappers(wrapMethodArgs, wrapCallbackArgs dnode.Wrapper, runMethod, runCallback dnode.Runner, onError func(error)) {
	s.dnode.WrapMethodArgs = wrapMethodArgs
	s.dnode.WrapCallbackArgs = wrapCallbackArgs
	s.dnode.RunMethod = runMethod
	s.dnode.RunCallback = runCallback
	s.dnode.OnError = onError
}

// HandleFunc registers the handler for the given method.
// If a handler already exists for method, HandleFunc panics.
func (s *Server) HandleFunc(method string, handler interface{}) {
	s.dnode.HandleFunc(method, handler)
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
	c.properties["localKite"] = s.properties["localKite"]
	return c
}

func (s *Server) Properties() map[string]interface{} {
	return s.properties
}

func (s *Server) OnConnect(handler func(*Client)) {
	s.onConnectHandlers = append(s.onConnectHandlers, handler)
}

func (s *Server) OnDisconnect(handler func(*Client)) {
	s.onDisconnectHandlers = append(s.onDisconnectHandlers, handler)
}

func (s *Server) callOnConnectHandlers(c *Client) {
	for _, handler := range s.onConnectHandlers {
		handler(c)
	}
}

func (s *Server) callOnDisconnectHandlers(c *Client) {
	for _, handler := range s.onDisconnectHandlers {
		handler(c)
	}
}
