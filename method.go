package kite

// Objects implementing the Handler interface can be registered to a method.
// The returned result must be marshalable with json package.
type Handler interface {
	ServeKite(*Request) (result interface{}, err error)
}

// HandlerFunc is a type adapter to allow the use of ordinary functions as
// Kite handlers. If h is a function with the appropriate signature,
// HandlerFunc(h) is a Handler object that calls h.
type HandlerFunc func(*Request) (result interface{}, err error)

// ServeKite calls h(r)
func (h HandlerFunc) ServeKite(r *Request) (interface{}, error) {
	return h(r)
}

// Method defines a method and the Handler it is bind to.
type Method struct {
	// name is the method name
	name string

	// handler contains the related Handler
	handler Handler

	// authenticate defines if a given authenticator function is enabled for
	// the given auth type in the request.
	authenticate bool
}

// DisableAuthentication disables authentication check for this method.
func (m *Method) DisableAuthentication() *Method {
	m.authenticate = false
	return m
}

// Handle registers the handler for the given method. The handler is called
// when a method call is received from a Kite.
func (k *Kite) Handle(method string, handler Handler) *Method {
	return k.addHandle(method, handler)
}

// HandleFunc registers a handler to run when a method call is received from a
// Kite. It returns a *Method option to further modify certain options on a
// method call
func (k *Kite) HandleFunc(method string, handler HandlerFunc) *Method {
	return k.addHandle(method, handler)
}

// addHandle is an internal method to add a handler
func (k *Kite) addHandle(method string, handler Handler) *Method {
	authenticate := true
	if k.Config.DisableAuthentication {
		authenticate = false
	}

	m := &Method{
		name:         method,
		handler:      handler,
		authenticate: authenticate,
	}

	k.handlers[method] = m
	return m
}
