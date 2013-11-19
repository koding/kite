package kite

import (
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/kodingkey"
	"koding/newkite/token"
)

type HandlerFunc func(*Request) (response interface{}, err error)

type Request struct {
	Method         string
	Args           *dnode.Partial
	LocalKite      *Kite
	RemoteKite     *RemoteKite
	Username       string
	Authentication *callAuthentication
	RemoteAddr     string
}

// HandleDnodeMessage implemets dnode.MessageHandler interface.
func (k *Kite) HandleDnodeMessage(msg *dnode.Message, dn *dnode.Dnode, tr dnode.Transport) (err error) {
	var (
		args     []*dnode.Partial
		options  CallOptions
		response dnode.Function
	)

	// Parse dnode method arguments [options, response]
	err = msg.Arguments.Unmarshal(&args)
	if err != nil {
		return err
	}

	// Parse options argument
	err = args[0].Unmarshal(&options)
	if err != nil {
		return err
	}

	// Parse response callback if present
	if len(args) > 1 && args[1] != nil {
		err = args[1].Unmarshal(&response)
		if err != nil {
			return err
		}
	}

	// Trust the Kite if we have initiated the connection.
	// Otherwise try to authenticate the user.
	// RemoteAddr() returns "" if this is an outgoing connection.
	if tr.RemoteAddr() != "" {
		if err = k.authenticateUser(&options); err != nil {
			return err
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
	}

	request := &Request{
		Method:         fmt.Sprint(msg.Method),
		Args:           options.WithArgs,
		LocalKite:      k,
		RemoteKite:     properties["remoteKite"].(*RemoteKite),
		RemoteAddr:     tr.RemoteAddr(),
		Username:       options.Kite.Username, // authenticateUser() sets it.
		Authentication: &options.Authentication,
	}

	// We need this information on disconnect
	properties["username"] = request.Username
	properties["kiteID"] = request.RemoteKite.Kite.ID

	// Find handler function
	handler := k.Handlers[request.Method]
	if handler == nil {
		err = fmt.Errorf("Unknown method: %s", request.Method)
		return response(err.Error(), nil)
	}

	// Call the handler
	result, err := handler(request)

	// There is not a callback if RemoteKite.Go() is used.
	if response == nil {
		return nil
	}

	// Send an error response.
	if err != nil {
		return response(err.Error(), nil)
	}

	// Send the result back.
	return response(nil, result)
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
