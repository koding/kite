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
	request, responseCallback, err := m.kite.parseRequest(method, args, tr, true)
	if err != nil {
		m.kite.Log.Notice("Did not understand request: %s", err)
		return
	}

	result, err := m.handler(request)
	if responseCallback == nil {
		return
	}

	if err != nil {
		err = responseCallback(err.Error(), result)
	} else {
		err = responseCallback(nil, result)
	}

	if err != nil {
		m.kite.Log.Error(err.Error())
	}
}

type HandlerFunc func(*Request) (response interface{}, err error)

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
	req, _, err := k.parseRequest(method, args, tr, false)
	if err != nil {
		k.Log.Notice("Did not understand callback message: %s", err)
		return
	}

	c(req)
}

// parseRequest is used to read a dnode message.
// It is called when a method or callback is received.
func (k *Kite) parseRequest(method string, arguments *dnode.Partial, tr dnode.Transport, authenticate bool) (
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

	// Trust the Kite if we have initiated the connection.
	// Otherwise try to authenticate the user.
	// RemoteAddr() returns "" if this is an outgoing connection.
	// Also we do not need to authenticate requests when a callback method is received.
	if authenticate && tr.RemoteAddr() != "" {
		if err = k.authenticateUser(&options); err != nil {
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
		Username:       options.Kite.Username, // authenticateUser() sets it.
		Authentication: &options.Authentication,
	}

	return
}

// authenticateUser tries to authenticate the user by selecting appropriate
// authenticator function.
func (k *Kite) authenticateUser(options *CallOptions) error {
	f := k.Authenticators[options.Authentication.Type]
	if f == nil {
		return fmt.Errorf("Unknown authentication type: %s", options.Authentication.Type)
	}

	return f(options)
}

// AuthenticateFromToken is the default Authenticator for Kite.
func (k *Kite) AuthenticateFromToken(options *CallOptions) error {
	key, err := kodingkey.FromString(k.KodingKey)
	if err != nil {
		return fmt.Errorf("Invalid Koding Key: %s", k.KodingKey)
	}

	tkn, err := token.DecryptString(options.Authentication.Key, key)
	if err != nil {
		return fmt.Errorf("Invalid token: %s", options.Authentication.Key)
	}

	if !tkn.IsValid(k.ID) {
		return fmt.Errorf("Invalid token: %s", tkn)
	}

	options.Kite.Username = tkn.Username

	return nil
}

// AuthenticateFromToken authenticates user from Koding Key.
// Kontrol makes requests with a Koding Key.
func (k *Kite) AuthenticateFromKodingKey(options *CallOptions) error {
	if options.Authentication.Key != k.KodingKey {
		return fmt.Errorf("Invalid Koding Key")
	}

	// Set the username if missing.
	if options.Kite.Username == "" && k.Username != "" {
		options.Kite.Username = k.Username
	}

	return nil
}
