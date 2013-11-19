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
	// The information about the kite that we are connecting to.
	protocol.Kite

	// A reference to the current Kite running.
	localKite *Kite

	// Credentials that we sent in each request.
	Authentication callAuthentication

	// dnode RPC client that processes messages.
	Client *rpc.Client

	// A channel to notify waiters on Call() or Go() when we disconnect.
	disconnect chan bool
}

// NewRemoteKite returns a pointer to a new RemoteKite. The returned instance
// is not connected. You have to call Dial() or DialForever() before calling
// Call() and Go() methods.
func (k *Kite) NewRemoteKite(kite protocol.Kite, auth callAuthentication) *RemoteKite {
	r := &RemoteKite{
		Kite:           kite,
		localKite:      k,
		Authentication: auth,
		Client:         k.Server.NewClientWithHandlers(),
		disconnect:     make(chan bool),
	}

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

// CallOptions is the type of first argument in the dnode message.
// Second argument is a callback function.
// It is used when unmarshalling a dnode message.
type CallOptions struct {
	// Arguments to the method
	WithArgs       *dnode.Partial     `json:"withArgs"`
	Kite           protocol.Kite      `json:"kite"`
	Authentication callAuthentication `json:"authentication"`
}

// callOptionsOut is the same structure with CallOptions.
// It is used when marshalling a dnode message.
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
			Kite:           r.localKite.Kite,
			Authentication: r.Authentication,
		},
	}
}

type callAuthentication struct {
	// Type can be "kodingKey", "token" or "sessionID" for now.
	Type string `json:"type"`
	Key  string `json:"key"`
}

type response struct {
	Result *dnode.Partial
	Err    error
}

// Call makes a blocking method call to the server.
// Send a callback function and waits until it is called or connection drops.
// Returns the result and the error as the other side sends.
func (r *RemoteKite) Call(method string, args interface{}) (result *dnode.Partial, err error) {
	response := <-r.Go(method, args)
	return response.Result, response.Err
}

// Go makes an unblocking method call to the server.
func (r *RemoteKite) Go(method string, args interface{}) chan *response {
	// We will return this channel to the caller.
	// It can wait on this channel to get the response.
	responseChan := make(chan *response, 1)

	// The response value that will be sent to the returned channel.
	theResponse := new(response)

	// Buffered channel for waiting response from server.
	// Strobes when response callback is called.
	done := make(chan bool, 1)

	// To clean the sent callback after response is received.
	// Send/Receive in a channel to prevent race condition because
	// the callback is run in a separate goroutine.
	removeCallback := make(chan uint64, 1)

	// This is the callback function sent to the server.
	// The caller of the Call() is blocked until the server calls this callback function.
	// This function does not return anything but sets "result" and "err" variables
	// in upper scope.
	responseCallback := func(arguments *dnode.Partial) {
		var (
			// Arguments to our response callback It is a slice of length 2.
			// The first argument is the error string,
			// the second argument is the result.
			responseArgs []*dnode.Partial

			// First argument
			err error

			// Second argument
			result *dnode.Partial
		)

		// Notify that the callback is finished.
		defer func() {
			// Sets the response from outer scope.
			theResponse.Result = result
			theResponse.Err = err

			done <- true
		}()

		// Remove the callback function from the map so we do not
		// consume memory for unused callbacks.
		id := <-removeCallback
		r.Client.RemoveCallback(id)

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

		var errorString string
		err = responseArgs[0].Unmarshal(&errorString)
		if err != nil {
			return
		}
		if errorString != "" {
			err = errors.New(errorString)
		}
	}

	// Send the method with callback to the server.
	callbacks, err := r.Client.Call(method, r.makeOptions(args), dnode.Callback(responseCallback))
	if err != nil {
		responseChan <- &response{nil, err}
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

	// Waits until the response callback is finished or connection is disconnected.
	go func() {
		select {
		case <-done:
			responseChan <- theResponse
		case <-r.disconnect:
			responseChan <- &response{nil, errors.New("Client disconnected")}
		}
	}()

	return responseChan
}
