package rpc

import (
	"code.google.com/p/go.net/websocket"
	"errors"
	"reflect"
)

type Server struct {
	websocket.Server
	handlers     map[string]interface{}
	OnConnect    func(*Client)
	OnDisconnect func(*Client)
}

func NewServer() *Server {
	s := &Server{
		handlers: make(map[string]interface{}),
	}
	s.Handler = s.handleWS
	return s
}

func (s *Server) HandleFunc(method string, handler interface{}) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		panic(errors.New("handler is not a func"))
	}

	s.handlers[method] = handler
}

func (s *Server) handleWS(ws *websocket.Conn) {
	defer ws.Close()

	c := newClient(ws)

	if s.OnConnect != nil {
		s.OnConnect(c)
	}

	// Initialize dnode with registered methods
	for method, handler := range s.handlers {
		c.dnode.HandleFunc(method, handler)
	}

	c.run()

	if s.OnDisconnect != nil {
		s.OnDisconnect(c)
	}
}
