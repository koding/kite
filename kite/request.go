package kite

import (
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/kodingkey"
	"koding/newkite/token"
)

// kiteMethod implements dnode.Handler.
type kiteMethod struct {
	kite    *Kite
	method  string
	handler HandlerFunc
}

// WrapArgs is called before a Callback is sent to remote in order to wrap arguments.
func (m kiteMethod) WrapArgs(args []interface{}, tr dnode.Transport) []interface{} {
	return []interface{}{&callOptionsOut{
		WithArgs: args,
		CallOptions: CallOptions{
			Kite: m.kite.Kite,
		},
	}}
}

// Call is called when a method is received from remote.
func (m kiteMethod) Call(method string, args *dnode.Partial, tr dnode.Transport) {
	request, responseCallback, err := m.kite.parseRequest(method, args, tr)
	if err != nil {
		m.kite.Log.Notice("Did not understand request: %s", err)
		return
	}

	err = request.authenticate()
	if kiteErr, ok := err.(*Error); ok && kiteErr.Type == "authenticationError" {
		err = responseCallback(kiteErr, nil)
	}
	if err != nil {
		m.kite.Log.Error(err.Error())
		return
	}

	var result interface{}

	// Wrap handler func.
	handler := func() { result, err = m.handler(request) }

	// Recover dnode argument errors.
	// The caller can use functions like MustString(), MustSlice()...
	// without the fear of panic.
	argumentErr := recoverArgumentError(handler)

	if responseCallback == nil {
		return
	}

	// Prepare error argument.
	if argumentErr != nil {
		err = &Error{"argumentError", argumentErr.Error()}
	} else if err != nil {
		// Convert all errors to kite.Error type.
		if _, ok := err.(*Error); !ok {
			err = &Error{"serverError", err.Error()}
		}
	}

	// Call response callback function.
	if err = responseCallback(err, result); err != nil {
		m.kite.Log.Error(err.Error())
	}
}

// Error is the type of the first argument in response callback.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprint("Kite error: %s: %s", e.Type, e.Message)
}

// recoverArgumentError takes a function and tries to recover a dnode.ArgumentError
// if it panics.
func recoverArgumentError(f func()) (err *dnode.ArgumentError) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		var ok bool
		if err, ok = r.(*dnode.ArgumentError); !ok {
			panic(r)
		}
	}()

	f()

	return
}

type HandlerFunc func(*Request) (result interface{}, err error)

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (k *Kite) HandleFunc(method string, handler HandlerFunc) {
	k.server.Handle(method, kiteMethod{k, method, handler})
}

type Request struct {
	Method         string
	Args           *dnode.Partial
	LocalKite      *Kite
	RemoteKite     *RemoteKite
	Username       string
	Authentication *callAuthentication
	RemoteAddr     string
}

type Callback func(r *Request)

func (c Callback) MarshalJSON() ([]byte, error) {
	return []byte(`"[Function]"`), nil
}

func (c Callback) WrapArgs(args []interface{}, tr dnode.Transport) []interface{} {
	return []interface{}{&callOptionsOut{
		WithArgs: args,
		CallOptions: CallOptions{
			Kite: tr.Properties()["localKite"].(*Kite).Kite,
		},
	}}
}

// Call is called when a callback method call is received from remote.
func (c Callback) Call(method string, args *dnode.Partial, tr dnode.Transport) {
	k := tr.Properties()["localKite"].(*Kite)
	req, _, err := k.parseRequest(method, args, tr)
	if err != nil {
		k.Log.Notice("Did not understand callback message: %s. method: %q args: %q", err, method, args)
		return
	}

	recoverArgumentError(func() { c(req) })
}

// parseRequest is used to read a dnode message.
// It is called when a method or callback is received.
func (k *Kite) parseRequest(method string, arguments *dnode.Partial, tr dnode.Transport) (
	request *Request, response dnode.Function, err error) {

	// Parse dnode method arguments [options, response]
	args, err := arguments.Slice()
	if err != nil {
		return
	}
	if len(args) != 1 && len(args) != 2 {
		return nil, nil, errors.New("Invalid number of arguments")
	}

	// Parse options argument
	var options CallOptions
	if err = args[0].Unmarshal(&options); err != nil {
		return
	}

	// Parse response callback if present
	if len(args) > 1 && args[1] != nil {
		if err = args[1].Unmarshal(&response); err != nil {
			return
		}
	}

	// Properties about the client...
	properties := tr.Properties()

	// Create a new RemoteKite instance to pass it to the handler, so
	// the handler can call methods on the other site on the same connection.
	if properties["remoteKite"] == nil {
		// Do not create a new RemoteKite on every request,
		// cache it in Transport.Properties().
		client := tr.(*rpc.Client) // We only have a dnode/rpc.Client for now.
		remoteKite := k.newRemoteKiteWithClient(options.Kite, options.Authentication, client)
		properties["remoteKite"] = remoteKite

		// Notify Kite.OnConnect handlers.
		k.notifyRemoteKiteConnected(remoteKite)
	}

	request = &Request{
		Method:         method,
		Args:           options.WithArgs,
		LocalKite:      k,
		RemoteKite:     properties["remoteKite"].(*RemoteKite),
		RemoteAddr:     tr.RemoteAddr(),
		Username:       options.Kite.Username,
		Authentication: &options.Authentication,
	}

	return
}

// authenticate tries to authenticate the user by selecting appropriate
// authenticator function.
func (r *Request) authenticate() error {
	// Trust the Kite if we have initiated the connection.
	// RemoteAddr() returns "" if this is an outgoing connection.
	if r.RemoteAddr == "" {
		return nil
	}

	// Select authenticator function.
	f := r.LocalKite.Authenticators[r.Authentication.Type]
	if f == nil {
		return &Error{
			Type:    "authenticationError",
			Message: fmt.Sprint("Unknown authentication type: %s", r.Authentication.Type),
		}
	}

	// Call authenticator function.
	err := f(r)
	if err != nil {
		return &Error{
			Type:    "authenticationError",
			Message: err.Error(),
		}
	}

	// Fix username of the remote Kite if it is invalid.
	r.RemoteKite.Kite.Username = r.Username

	return nil
}

// AuthenticateFromToken is the default Authenticator for Kite.
func (k *Kite) AuthenticateFromToken(r *Request) error {
	key, err := kodingkey.FromString(k.KodingKey)
	if err != nil {
		return fmt.Errorf("Invalid Koding Key: %s", k.KodingKey)
	}

	tkn, err := token.DecryptString(r.Authentication.Key, key)
	if err != nil {
		return fmt.Errorf("Invalid token: %s", r.Authentication.Key)
	}

	if !tkn.IsValid(k.ID) {
		return fmt.Errorf("Invalid token: %s", tkn)
	}

	r.Username = tkn.Username

	return nil
}

// AuthenticateFromToken authenticates user from Koding Key.
// Kontrol makes requests with a Koding Key.
func (k *Kite) AuthenticateFromKodingKey(r *Request) error {
	if r.Authentication.Key != k.KodingKey {
		return fmt.Errorf("Invalid Koding Key")
	}

	// Set the username if missing.
	if r.RemoteKite.Username == "" && k.Username != "" {
		r.RemoteKite.Username = k.Username
	}

	return nil
}
