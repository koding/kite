package kite

// Objects implementing the Handler interface can be registered to a particular
// method. The returned result must be Marshalable with json package.
type Handler interface {
	ServeKite(*Request) (result interface{}, err error)
}

// The HandlerFunc type is an adapter to allow the use of ordinary functions as
// Kite handlers. If h is a function with the appropriate signature,
// HandlerFunc(h) is a Handler object that calls h.
type HandlerFunc func(*Request) (result interface{}, err error)

// ServeKite calls h(r)
func (h HandlerFunc) ServeKite(r *Request) (interface{}, error) {
	return h(r)
}

// Method defines a method and the Handlerfunc it is bind to.
type Method struct {
	// name is the method name
	name string

	// handler contains the relates Handler
	handler Handler
}

func (k *Kite) Handle(method string, handler Handler) *Method {
	m := &Method{
		name:    method,
		handler: handler,
	}

	k.handlers[method] = m
	return m
}

// HandleFunc registers a handler to run when a method call is received from a
// Kite. It returns a *Method option to further modify certain options on a
// method call
func (k *Kite) HandleFunc(method string, handler HandlerFunc) *Method {
	m := &Method{
		name:    method,
		handler: handler,
	}

	k.handlers[method] = m
	return m
}
