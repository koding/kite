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
	// name is the method name. Unnamed methods can exist
	name string

	// handler contains the related Handler for the given method
	handlers []Handler

	// authenticate defines if a given authenticator function is enabled for
	// the given auth type in the request.
	authenticate bool
}

// DisableAuthentication disables authentication check for this method.
func (m *Method) DisableAuthentication() *Method {
	m.authenticate = false
	return m
}

// prepend is an internal method that's prepends the given handler to the
// method handler slice
func (m *Method) prepend(handler Handler) {
	m.handlers = append(m.handlers, nil)
	copy(m.handlers[1:], m.handlers[0:])
	m.handlers[0] = handler
}

// PreHandler adds a new kite handler which is executed before the method.
func (m *Method) PreHandle(handler Handler) *Method {
	m.prepend(handler)
	return m
}

// PreHandlerFunc adds a new kite handlerfunc which is executed before the
// method.
func (m *Method) PreHandleFunc(handler HandlerFunc) *Method {
	return m.PreHandle(handler)
}

// PostHandle adds a new kite handler which is executed after the method.
func (m *Method) PostHandle(handler Handler) *Method {
	m.handlers = append(m.handlers, handler)
	return m
}

// PostHandlerFunc adds a new kite handlerfunc which is executed before the
// method.
func (m *Method) PostHandleFunc(handler HandlerFunc) *Method {
	return m.PostHandle(handler)
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
		handlers:     []Handler{handler},
		authenticate: authenticate,
	}

	k.handlers[method] = m
	return m
}

// PreHandle registers an handler which is executed before a kite.Handler
// method is executed. Calling PreHandle multiple times registers multiple
// handlers. A non-error return triggers the execution of the next handler. The
// execution order is FIFO.
func (k *Kite) PreHandle(handler Handler) {
	k.preHandlers = append(k.preHandlers, handler)
}

// PreHandleFunc is the same as PreHandle. It accepts a HandlerFunc.
func (k *Kite) PreHandleFunc(handler HandlerFunc) {
	k.PreHandle(handler)
}

// PostHandle registers an handler which is executed after a kite.Handler
// method is executed. Calling PostHandler multiple times registers multiple
// handlers. A non-error return triggers the execution of the next handler. The
// execution order is FIFO.
func (k *Kite) PostHandle(handler Handler) {
	k.postHandlers = append(k.postHandlers, handler)
}

// PostHandleFunc is the same as PostHandle. It accepts a HandlerFunc.
func (k *Kite) PostHandleFunc(handler HandlerFunc) {
	k.PostHandle(handler)
}

// multiHandler is a type that satisifes the kite.Handler interface
type multiHandler []Handler

func (m multiHandler) ServeKite(r *Request) (interface{}, error) {
	for _, handler := range m {
		resp, err := handler.ServeKite(r)
		if err != nil {
			// exit only if there is a problem
			return nil, err
		}

		// save for next iteration
		r.Response = resp
	}

	return r.Response, nil
}
