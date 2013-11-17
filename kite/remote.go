package kite

import (
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/protocol"
	"strconv"
)

// RemoteKite is the client for communicating with another Kite.
// It has Call() and Go() methods for calling methods sync/async way.
type RemoteKite struct {
	protocol.Kite
	LocalKite      *Kite
	Authentication callAuthentication
	Client         *rpc.Client
	disconnect     chan bool
}

// NewRemoteKite returns a pointer to a new RemoteKite. The returned instance
// is not connected. You have to call Dial() or DialForever() before calling
// Call() and Go() methods.
func (k *Kite) NewRemoteKite(kite protocol.Kite, auth callAuthentication) *RemoteKite {
	r := &RemoteKite{
		Kite:           kite,
		LocalKite:      k,
		Authentication: auth,
		Client:         rpc.NewClient(),
		disconnect:     make(chan bool),
	}

	r.Client.Dnode.ExternalHandler = k
	r.Client.OnDisconnect(r.notifyDisconnect)
	return r
}

// newRemoteKiteWithClient returns a pointer to new RemoteKite instance.
// The client will be replaced with the given client.
// Used to give the Kite method handler a working RemoteKite to call methods
// on other side.
func (k *Kite) newRemoteKiteWithClient(kite protocol.Kite, auth callAuthentication, client *rpc.Client) *RemoteKite {
	r := k.NewRemoteKite(kite, auth)
	r.Client = client
	r.Client.OnDisconnect(r.notifyDisconnect)
	return r
}

// notifyDisconnect unblocks the caller of Call() on disconnect.
func (r *RemoteKite) notifyDisconnect() {
	// Unblocking send.
	select {
	case r.disconnect <- true:
	default:
	}
}

// Dial connects to the remote Kite. Returns error if it can't.
func (r *RemoteKite) Dial() (err error) {
	addr := r.Kite.Addr()
	log.Info("Dialling %s", addr)
	return r.Client.Dial("ws://" + addr + "/dnode")
}

// Dial connects to the remote Kite. If it can't connect, it retries indefinitely.
func (r *RemoteKite) DialForever() {
	addr := r.Kite.Addr()
	log.Info("Dialling %s", addr)
	r.Client.DialForever("ws://" + addr + "/dnode")
}

// CallOptions is the first argument in the dnode message.
// Second argument is a callback function.
type CallOptions struct {
	// Arguments to the method
	WithArgs       *dnode.Partial     `json:"withArgs"`
	Kite           protocol.Kite      `json:"kite"`
	Authentication callAuthentication `json:"authentication"`
}

type callOptionsOut struct {
	CallOptions
	// Override this when sending because args will not be a *dnode.Partial.
	WithArgs interface{} `json:"withArgs"`
}

// That's what we send as a first argument in dnode message.
func (r *RemoteKite) makeOptions(args interface{}) *callOptionsOut {
	return &callOptionsOut{
		WithArgs: args,
		CallOptions: CallOptions{
			Kite:           r.LocalKite.Kite,
			Authentication: r.Authentication,
		},
	}
}

type callAuthentication struct {
	// Type can be "kodingKey", "token" or "sessionID" for now.
	Type string `json:"type"`
	Key  string `json:"key"`
}

// Go makes an unblocking mehtod call to the server.
func (r *RemoteKite) Go(method string, args interface{}) error {
	options := r.makeOptions(args)
	_, err := r.Client.Call(method, options)
	return err
}

// Call makes a blocking method call to the server.
// Send a callback function and waits until it is called or connection drops.
// Returns the result and the error as the other side sends.
func (r *RemoteKite) Call(method string, args interface{}) (result *dnode.Partial, err error) {
	options := r.makeOptions(args)

	// Buffered channel for waiting response from server
	done := make(chan bool, 1)

	// To clean the sent callback after response is received.
	// Send/Receive in a channel to prevent race condition because
	// the callback is run in a seperate goroutine.
	removeCallback := make(chan uint64, 1)

	// This is the callback function sent to the server.
	// The caller of the Call() is blocked until the server calls this callback function.
	// This function does not return anything but sets "result" and "err" vaiables
	// in upper scope.
	responseCallback := func(arguments *dnode.Partial) {
		// Unblock the caller.
		defer func() { done <- true }()

		// Remove the callback function from the map so we do not
		// consume memory for unused callbacks.
		id := <-removeCallback
		r.Client.Dnode.RemoveCallback(id)

		var (
			// Arguments to our response callback It is a slice of length 2.
			// The first argument is the error string,
			// the second argument is the result.
			responseArgs []*dnode.Partial

			// The first argument mentioned above.
			responseError string
		)

		err = arguments.Unmarshal(&responseArgs)
		if err != nil {
			return
		}

		// We must always get an error and a result argument.
		if len(responseArgs) != 2 {
			err = fmt.Errorf("Invalid response args: %s", string(arguments.Raw))
			return
		}

		// The second argument is the our result, set it on upper scope.
		result = responseArgs[1]

		// This is error argument. Unmarshal panics if it is null.
		if responseArgs[0] == nil {
			return
		}

		err = responseArgs[0].Unmarshal(&responseError)
		if err != nil {
			return
		}

		// Set the err argument on upper scope.
		err = errors.New(responseError)
	}

	// Send the method with callback to the server.
	callbacks, err := r.Client.Call(method, options, dnode.Callback(responseCallback))
	if err != nil {
		return nil, err
	}

	// Find the callback number to be deleted after response is received.
	var max uint64 = 0
	for id, _ := range callbacks {
		i, _ := strconv.ParseUint(id, 10, 64)
		if i > max {
			max = i
		}
	}
	removeCallback <- max

	// Block until response callback is called or connection disconnect.
	select {
	case <-done:
		return result, err
	case <-r.disconnect:
		return nil, errors.New("Client disconnected")
	}
}
