package kite

import (
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
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

func (k *Kite) parseRequest(msg *dnode.Message, tr dnode.Transport) (request *Request, response dnode.Function, err error) {
	var (
		args    []*dnode.Partial
		options CallOptions
	)

	// Parse dnode method arguments [options, response]
	if err = msg.Arguments.Unmarshal(&args); err != nil {
		return
	}

	// Parse options argument
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
	if tr.RemoteAddr() != "" {
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
	}

	request = &Request{
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

	return
}
