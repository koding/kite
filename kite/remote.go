package kite

import (
	"errors"
	"fmt"
	"github.com/op/go-logging"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/protocol"
	"strconv"
	"sync"
	"time"
)

const DefaultTellTimeout = 4 * time.Second

// RemoteKite is the client for communicating with another Kite.
// It has Tell() and Go() methods for calling methods sync/async way.
type RemoteKite struct {
	// The information about the kite that we are connecting to.
	protocol.Kite

	// A reference to the current Kite running.
	localKite *Kite

	// A reference to the Kite's logger for easy access.
	Log *logging.Logger

	// Credentials that we sent in each request.
	Authentication callAuthentication

	// dnode RPC client that processes messages.
	client *rpc.Client

	// To signal waiters of Go() on disconnect.
	disconnect chan bool

	// Duration to wait reply from remote when making a request with Tell().
	tellTimeout time.Duration
}

// NewRemoteKite returns a pointer to a new RemoteKite. The returned instance
// is not connected. You have to call Dial() or DialForever() before calling
// Tell() and Go() methods.
func (k *Kite) NewRemoteKite(kite protocol.Kite, auth callAuthentication) *RemoteKite {
	r := &RemoteKite{
		Kite:           kite,
		localKite:      k,
		Log:            k.Log,
		Authentication: auth,
		client:         k.server.NewClientWithHandlers(),
		disconnect:     make(chan bool),
	}
	r.SetTellTimeout(DefaultTellTimeout)

	// We need a reference to the local kite when a method call is received.
	r.client.Properties()["localKite"] = k

	r.OnConnect(func() {
		if r.Authentication.ValidUntil == nil {
			return
		}

		// Start a goroutine that will renew the token before it expires.
		go r.tokenRenewer()
	})

	var m sync.Mutex
	r.OnDisconnect(func() {
		m.Lock()
		close(r.disconnect)
		r.disconnect = make(chan bool)
		m.Unlock()
	})

	return r
}

// newRemoteKiteWithClient returns a pointer to new RemoteKite instance.
// The client will be replaced with the given client.
// Used to give the Kite method handler a working RemoteKite to call methods
// on other side.
func (k *Kite) newRemoteKiteWithClient(kite protocol.Kite, auth callAuthentication, client *rpc.Client) *RemoteKite {
	r := k.NewRemoteKite(kite, auth)
	r.client = client
	r.client.Properties()["localKite"] = k
	return r
}

// SetTellTimeout sets the timeout duration for requests made with Tell().
func (r *RemoteKite) SetTellTimeout(d time.Duration) { r.tellTimeout = d }

// Dial connects to the remote Kite. Returns error if it can't.
func (r *RemoteKite) Dial() (err error) {
	addr := r.Kite.Addr()
	r.Log.Info("Dialing remote kite: [%s %s]", r.Kite.Name, addr)
	return r.client.Dial("ws://" + addr + "/dnode")
}

// Dial connects to the remote Kite. If it can't connect, it retries indefinitely.
func (r *RemoteKite) DialForever() {
	addr := r.Kite.Addr()
	r.Log.Info("Dialing remote kite: [%s %s]", r.Kite.Name, addr)
	r.client.DialForever("ws://" + addr + "/dnode")
}

func (r *RemoteKite) Close() {
	r.client.Close()
}

// OnConnect registers a function to run on connect.
func (r *RemoteKite) OnConnect(handler func()) {
	r.client.OnConnect(handler)
}

// OnDisconnect registers a function to run on disconnect.
func (r *RemoteKite) OnDisconnect(handler func()) {
	r.client.OnDisconnect(handler)
}

func (r *RemoteKite) tokenRenewer() {
	for {
		// Token will be renewed before it expires.
		renewTime := r.Authentication.ValidUntil.Add(-30 * time.Second)
		select {
		case <-time.After(renewTime.Sub(time.Now().UTC())):
			if err := r.renewTokenUntilDisconnect(); err != nil {
				return
			}
		case <-r.disconnect:
			return
		}
	}
}

// renewToken retries until the request is successful or disconnect.
func (r *RemoteKite) renewTokenUntilDisconnect() error {
	const retryInterval = 10 * time.Second

	if err := r.renewToken(); err == nil {
		return nil
	}

loop:
	for {
		select {
		case <-time.After(retryInterval):
			if err := r.renewToken(); err != nil {
				r.Log.Error("error: %s Cannot renew token for Kite: %s I will retry in %d seconds...", err.Error(), r.Kite.ID, retryInterval)
				continue
			}

			break loop
		case <-r.disconnect:
			return errors.New("disconnect")
		}
	}

	return nil
}

func (r *RemoteKite) renewToken() error {
	tkn, err := r.localKite.Kontrol.GetToken(&r.Kite)
	if err != nil {
		return err
	}

	validUntil := time.Now().UTC().Add(time.Duration(tkn.TTL) * time.Second)
	r.Authentication.Key = tkn.Key
	r.Authentication.ValidUntil = &validUntil

	return nil
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
	Type       string     `json:"type"`
	Key        string     `json:"key"`
	ValidUntil *time.Time `json:"-"`
}

type response struct {
	Result *dnode.Partial
	Err    error
}

// Tell makes a blocking method call to the server.
// Waits until the callback function is called by the other side and
// returns the result and the error.
func (r *RemoteKite) Tell(method string, args interface{}) (result *dnode.Partial, err error) {
	return r.TellWithTimeout(method, args, 0)
}

// TellWithTimeout does the same thing with Tell() method except it takes an
// extra argument that is the timeout for waiting reply from the remote Kite.
// If timeout is given 0, the behavior is same as Tell().
func (r *RemoteKite) TellWithTimeout(method string, args interface{}, timeout time.Duration) (result *dnode.Partial, err error) {
	response := <-r.GoWithTimeout(method, args, timeout)
	return response.Result, response.Err
}

// Go makes an unblocking method call to the server.
// It returns a channel that the caller can wait on it to get the response.
func (r *RemoteKite) Go(method string, args interface{}) chan *response {
	return r.GoWithTimeout(method, args, 0)
}

// GoWithTimeout does the same thing with Go() method except it takes an
// extra argument that is the timeout for waiting reply from the remote Kite.
// If timeout is given 0, the behavior is same as Go().
func (r *RemoteKite) GoWithTimeout(method string, args interface{}, timeout time.Duration) chan *response {
	// We will return this channel to the caller.
	// It can wait on this channel to get the response.
	r.Log.Debug("Telling method [%s] on kite [%s]", method, r.Name)
	responseChan := make(chan *response, 1)

	r.send(method, args, timeout, responseChan)

	return responseChan
}

// send sends the method with callback to the server.
func (r *RemoteKite) send(method string, args interface{}, timeout time.Duration, responseChan chan *response) {
	// To clean the sent callback after response is received.
	// Send/Receive in a channel to prevent race condition because
	// the callback is run in a separate goroutine.
	removeCallback := make(chan uint64, 1)

	// When a callback is called it will send the response to this channel.
	doneChan := make(chan *response, 1)

	opts := r.makeOptions(args)
	cb := r.makeResponseCallback(doneChan, removeCallback)

	// BUG: This sometimes does not return an error, even if the remote
	// kite is disconnected. I could not find out why.
	// Timeout below in goroutine saves us in this case.
	callbacks, err := r.client.Call(method, opts, cb)
	if err != nil {
		responseChan <- &response{
			Result: nil,
			Err: fmt.Errorf("Telling method [%s] on [%s] error: %s",
				method, r.Kite.Name, err),
		}
		return
	}

	// Use default timeout from r (RemoteKite) if zero.
	if timeout == 0 {
		timeout = r.tellTimeout
	}

	// Waits until the response has came or the connection has disconnected.
	go func() {
		select {
		case resp := <-doneChan:
			responseChan <- resp
		case <-r.disconnect:
			responseChan <- &response{nil, errors.New("Client disconnected")}
		case <-time.After(timeout):
			responseChan <- &response{nil, errors.New("Timeout")}

			// Remove the callback function from the map so we do not
			// consume memory for unused callbacks.
			if id, ok := <-removeCallback; ok {
				r.client.RemoveCallback(id)
			}
		}
	}()

	sendCallbackID(callbacks, removeCallback)
}

// sendCallbackID send the callback number to be deleted after response is received.
func sendCallbackID(callbacks map[string]dnode.Path, ch chan uint64) {
	if len(callbacks) > 0 {
		// Find max callback ID.
		max := uint64(0)
		for id, _ := range callbacks {
			i, _ := strconv.ParseUint(id, 10, 64)
			if i > max {
				max = i
			}
		}

		ch <- max
	} else {
		close(ch)
	}
}

// makeResponseCallback prepares and returns a callback function sent to the server.
// The caller of the Tell() is blocked until the server calls this callback function.
// Sets theResponse and notifies the caller by sending to done channel.
func (r *RemoteKite) makeResponseCallback(doneChan chan *response, removeCallback <-chan uint64) Callback {
	return Callback(func(request *Request) {
		var (
			err    error          // First argument
			result *dnode.Partial // Second argument
		)

		// Notify that the callback is finished.
		defer func() { doneChan <- &response{result, err} }()

		// Remove the callback function from the map so we do not
		// consume memory for unused callbacks.
		if id, ok := <-removeCallback; ok {
			r.client.RemoveCallback(id)
		}

		// Arguments to our response callback:
		// The first argument is the error struct and
		// the second argument is the result.
		responseArgs := request.Args.MustSliceOfLength(2)

		result = responseArgs[1]

		// This is the error argument. Unmarshal panics if it is null.
		if responseArgs[0] == nil {
			return
		}

		// Read the error argument in response.
		var kiteErr *Error
		err = responseArgs[0].Unmarshal(kiteErr)
		if err != nil {
			return
		}

		err = kiteErr
	})
}
