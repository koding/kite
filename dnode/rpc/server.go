package rpc

import (
	"code.google.com/p/go.net/websocket"
	"errors"
	"reflect"
)

type Server struct {
	websocket.Server
	handlers map[string]interface{}
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
	c := newClient(ws)
	defer c.Close()

	// Initialize dnode with registered methods
	for method, handler := range s.handlers {
		c.dnode.HandleFunc(method, handler)
	}

	c.run()
}
