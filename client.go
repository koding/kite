package kite

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/sockjsclient"
	"gopkg.in/igm/sockjs-go.v2/sockjs"
)

var forever = backoff.NewExponentialBackOff()

func init() {
	forever.MaxElapsedTime = 365 * 24 * time.Hour // 1 year
}

// Client is the client for communicating with another Kite.
// It has Tell() and Go() methods for calling methods sync/async way.
type Client struct {
	// The information about the kite that we are connecting to.
	protocol.Kite
	muProt sync.Mutex // protects protocol.Kite access

	// A reference to the current Kite running.
	LocalKite *Kite

	// Credentials that we sent in each request.
	Auth *Auth

	// Should we reconnect if disconnected?
	Reconnect bool

	// SockJS base URL
	URL string

	// Should we process incoming messages concurrently or not? Default: true
	Concurrent bool

	// To signal waiters of Go() on disconnect.
	disconnect chan struct{}

	// SockJS session
	// TODO: replace this with a proper interface to support multiple
	// transport/protocols
	session sockjs.Session
	send    chan []byte

	// dnode scrubber for saving callbacks sent to remote.
	scrubber *dnode.Scrubber

	// Time to wait before redial connection.
	redialBackOff backoff.ExponentialBackOff

	// on connect/disconnect handlers are invoked after every
	// connect/disconnect.
	onConnectHandlers    []func()
	onDisconnectHandlers []func()

	// For protecting access over OnConnect and OnDisconnect handlers.
	m sync.RWMutex

	firstRequestHandlersNotified sync.Once
}

// callOptions is the type of first argument in the dnode message.
// It is used when unmarshalling a dnode message.
type callOptions struct {
	// Arguments to the method
	Kite             protocol.Kite  `json:"kite" dnode:"-"`
	Auth             *Auth          `json:"authentication"`
	WithArgs         *dnode.Partial `json:"withArgs" dnode:"-"`
	ResponseCallback dnode.Function `json:"responseCallback"`
}

// callOptionsOut is the same structure with callOptions.
// It is used when marshalling a dnode message.
type callOptionsOut struct {
	callOptions

	// Override this when sending because args will not be a *dnode.Partial.
	WithArgs []interface{} `json:"withArgs"`
}

// Authentication is used when connecting a Client.
type Auth struct {
	// Type can be "kiteKey", "token" or "sessionID" for now.
	Type string `json:"type"`
	Key  string `json:"key"`
}

// response is the type of the return value of Tell() and Go() methods.
type response struct {
	Result *dnode.Partial
	Err    error
}

// NewClient returns a pointer to a new Client. The returned instance
// is not connected. You have to call Dial() or DialForever() before calling
// Tell() and Go() methods.
func (k *Kite) NewClient(remoteURL string) *Client {
	r := &Client{
		LocalKite:     k,
		URL:           remoteURL,
		disconnect:    make(chan struct{}),
		redialBackOff: *forever,
		scrubber:      dnode.NewScrubber(),
		Concurrent:    true,
		send:          make(chan []byte, 512), // buffered
	}

	go r.sendHub()

	var m sync.Mutex
	r.OnDisconnect(func() {
		m.Lock()
		close(r.disconnect)
		r.disconnect = make(chan struct{})
		m.Unlock()
	})

	return r
}

func (c *Client) SetUsername(username string) {
	c.muProt.Lock()
	c.Kite.Username = username
	c.muProt.Unlock()
}

// Dial connects to the remote Kite. Returns error if it can't.
func (c *Client) Dial() (err error) {
	c.LocalKite.Log.Debug("Dialing '%s' kite: %s", c.Kite.Name, c.URL)

	if err := c.dial(); err != nil {
		return err
	}

	go c.run()

	return nil
}

// Dial connects to the remote Kite. If it can't connect, it retries
// indefinitely. It returns a channel to check if it's connected or not.
func (c *Client) DialForever() (connected chan bool, err error) {
	c.LocalKite.Log.Info("Dialing '%s' kite: %s", c.Kite.Name, c.URL)

	c.Reconnect = true
	connected = make(chan bool, 1) // This will be closed on first connection.
	go c.dialForever(connected)
	return
}

func (c *Client) dialForever(connectNotifyChan chan bool) {
	dial := func() error {
		if !c.Reconnect {
			return nil
		}
		return c.dial()
	}

	backoff.Retry(dial, &c.redialBackOff) // this will retry dial forever

	if connectNotifyChan != nil {
		close(connectNotifyChan)
	}

	go c.run()
}

func (c *Client) dial() (err error) {
	c.session, err = sockjsclient.ConnectWebsocketSession(c.URL)
	if err != nil {
		c.session = nil // explicitly set nil to avoid panicing when used the inside data.
		c.LocalKite.Log.Error(err.Error())
		return err
	}

	// Reset the wait time.
	c.redialBackOff.Reset()

	// Must be run in a goroutine because a handler may wait a response from
	// server.
	go c.callOnConnectHandlers()

	return nil
}

func (c *Client) RemoteAddr() string {
	if c.session == nil {
		return ""
	}

	websocketsession, ok := c.session.(*sockjsclient.WebsocketSession)
	if !ok {
		return ""
	}

	return websocketsession.RemoteAddr()
}

// randomStringLength is used to generate a session_id.
func randomStringLength(length int) string {
	size := (length * 6 / 8) + 1
	r := make([]byte, size)
	rand.Read(r)
	return base64.URLEncoding.EncodeToString(r)[:length]
}

// run consumes incoming dnode messages. Reconnects if necessary.
func (c *Client) run() {
	c.readLoop()

	// falls here when connection disconnects
	c.callOnDisconnectHandlers()

	if c.Reconnect {
		go c.dialForever(nil)
	}
}

// readLoop reads a message from websocket and processes it.
func (c *Client) readLoop() error {
	for {
		msg, err := c.receiveData()
		if err != nil {
			return err
		}

		processed := make(chan bool)
		go func(msg []byte, processed chan bool) {
			if err := c.processMessage(msg); err != nil {
				// don't log callback not found errors
				if _, ok := err.(dnode.CallbackNotFoundError); !ok {
					c.LocalKite.Log.Warning("error processing message err: %s message: %q", err.Error(), string(msg))
				}
			}
			close(processed)
		}(msg, processed)

		if !c.Concurrent {
			<-processed
		}
	}
}

// processMessage processes a single message and calls a handler or callback.
func (c *Client) processMessage(data []byte) (err error) {
	var (
		ok  bool
		msg dnode.Message
		m   *Method
	)

	// Call error handler.
	defer func() {
		if err != nil {
			onError(err)
		}
	}()

	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	sender := func(id uint64, args []interface{}) error {
		// do not name the error variable to "err" here, it's a trap for
		// shadowing variables
		_, errc := c.marshalAndSend(id, args)
		return errc
	}

	// Replace function placeholders with real functions.
	if err := dnode.ParseCallbacks(&msg, sender); err != nil {
		return err
	}

	// Find the handler function. Method may be string or integer.
	switch method := msg.Method.(type) {
	case float64:
		id := uint64(method)
		callback := c.scrubber.GetCallback(id)
		if callback == nil {
			err = dnode.CallbackNotFoundError{id, msg.Arguments}
			return err
		}
		c.runCallback(callback, msg.Arguments)
	case string:
		if m, ok = c.LocalKite.handlers[method]; !ok {
			err = dnode.MethodNotFoundError{method, msg.Arguments}
			return err
		}

		c.runMethod(m, msg.Arguments)
	default:
		return fmt.Errorf("Method is not string or integer: %+v (%T)", msg.Method, msg.Method)
	}
	return nil
}

func (c *Client) Close() {
	c.Reconnect = false
	if c.session != nil {
		c.session.Close(3000, "Go away!")
	}
}

// sendhub sends the msg received from the send channel to the remote client
func (c *Client) sendHub() {
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.LocalKite.Log.Warning("Send hub is closed")
				return
			}

			c.LocalKite.Log.Debug("Sending: %s", string(msg))
			if c.session == nil {
				c.LocalKite.Log.Error("not connected")
				continue
			}

			c.session.Send(string(msg))
		}
	}
}

// receiveData reads a message from session.
func (c *Client) receiveData() ([]byte, error) {
	if c.session == nil {
		return nil, errors.New("not connected")
	}

	msg, err := c.session.Recv()

	c.LocalKite.Log.Debug("Received : %s", msg)

	return []byte(msg), err
}

// OnConnect registers a function to run on connect.
func (c *Client) OnConnect(handler func()) {
	c.m.Lock()
	c.onConnectHandlers = append(c.onConnectHandlers, handler)
	c.m.Unlock()
}

// OnDisconnect registers a function to run on disconnect.
func (c *Client) OnDisconnect(handler func()) {
	c.m.Lock()
	c.onDisconnectHandlers = append(c.onDisconnectHandlers, handler)
	c.m.Unlock()
}

// callOnConnectHandlers runs the registered connect handlers.
func (c *Client) callOnConnectHandlers() {
	c.m.RLock()
	for _, handler := range c.onConnectHandlers {
		func() {
			defer recover()
			handler()
		}()
	}
	c.m.RUnlock()
}

// callOnDisconnectHandlers runs the registered disconnect handlers.
func (c *Client) callOnDisconnectHandlers() {
	c.m.RLock()
	for _, handler := range c.onDisconnectHandlers {
		func() {
			defer recover()
			handler()
		}()
	}
	c.m.RUnlock()
}

func (c *Client) wrapMethodArgs(args []interface{}, responseCallback dnode.Function) []interface{} {
	options := callOptionsOut{
		WithArgs: args,
		callOptions: callOptions{
			Kite:             *c.LocalKite.Kite(),
			Auth:             c.Auth,
			ResponseCallback: responseCallback,
		},
	}
	return []interface{}{options}
}

// Tell makes a blocking method call to the server.
// Waits until the callback function is called by the other side and
// returns the result and the error.
func (c *Client) Tell(method string, args ...interface{}) (result *dnode.Partial, err error) {
	return c.TellWithTimeout(method, 0, args...)
}

// TellWithTimeout does the same thing with Tell() method except it takes an
// extra argument that is the timeout for waiting reply from the remote Kite.
// If timeout is given 0, the behavior is same as Tell().
func (c *Client) TellWithTimeout(method string, timeout time.Duration, args ...interface{}) (result *dnode.Partial, err error) {
	response := <-c.GoWithTimeout(method, timeout, args...)
	return response.Result, response.Err
}

// Go makes an unblocking method call to the server.
// It returns a channel that the caller can wait on it to get the response.
func (c *Client) Go(method string, args ...interface{}) chan *response {
	return c.GoWithTimeout(method, 0, args...)
}

// GoWithTimeout does the same thing with Go() method except it takes an
// extra argument that is the timeout for waiting reply from the remote Kite.
// If timeout is given 0, the behavior is same as Go().
func (c *Client) GoWithTimeout(method string, timeout time.Duration, args ...interface{}) chan *response {
	// We will return this channel to the caller.
	// It can wait on this channel to get the response.
	c.LocalKite.Log.Debug("Telling method [%s] on kite [%s]", method, c.Name)
	responseChan := make(chan *response, 1)

	c.sendMethod(method, args, timeout, responseChan)

	return responseChan
}

// sendMethod wraps the arguments, adds a response callback,
// marshals the message and send it over the wire.
func (c *Client) sendMethod(method string, args []interface{}, timeout time.Duration, responseChan chan *response) {
	// To clean the sent callback after response is received.
	// Send/Receive in a channel to prevent race condition because
	// the callback is run in a separate goroutine.
	removeCallback := make(chan uint64, 1)

	// When a callback is called it will send the response to this channel.
	doneChan := make(chan *response, 1)

	cb := c.makeResponseCallback(doneChan, removeCallback, method, args)
	args = c.wrapMethodArgs(args, cb)

	// BUG: This sometimes does not return an error, even if the remote
	// kite is disconnected. I could not find out why.
	// Timeout below in goroutine saves us in this case.
	callbacks, err := c.marshalAndSend(method, args)
	if err != nil {
		responseChan <- &response{
			Result: nil,
			Err: &Error{
				Type:    "sendError",
				Message: err.Error(),
			},
		}
		return
	}

	// nil value of afterTimeout means no timeout, it will not selected in
	// select statement
	var afterTimeout <-chan time.Time
	if timeout > 0 {
		afterTimeout = time.After(timeout)
	}

	// Waits until the response has came or the connection has disconnected.
	go func() {
		select {
		case resp := <-doneChan:
			responseChan <- resp
		case <-c.disconnect:
			responseChan <- &response{
				nil,
				&Error{
					Type:    "disconnect",
					Message: "Remote kite has disconnected",
				},
			}
		case <-afterTimeout:
			responseChan <- &response{
				nil,
				&Error{
					Type:    "timeout",
					Message: fmt.Sprintf("No response to %q method in %s", method, timeout),
				},
			}

			// Remove the callback function from the map so we do not
			// consume memory for unused callbacks.
			if id, ok := <-removeCallback; ok {
				c.scrubber.RemoveCallback(id)
			}
		}
	}()

	sendCallbackID(callbacks, removeCallback)
}

// marshalAndSend takes a method and arguments, scrubs the arguments to create
// a dnode message, marshals the message to JSON and sends it over the wire.
func (c *Client) marshalAndSend(method interface{}, arguments []interface{}) (callbacks map[string]dnode.Path, err error) {
	callbacks = c.scrubber.Scrub(arguments)

	defer func() {
		if err != nil {
			c.removeCallbacks(callbacks)
		}
	}()

	// Do not encode empty arguments as "null", make it "[]".
	if arguments == nil {
		arguments = make([]interface{}, 0)
	}

	rawArgs, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}

	msg := dnode.Message{
		Method:    method,
		Arguments: &dnode.Partial{Raw: rawArgs},
		Callbacks: callbacks,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	c.send <- data
	return
}

// Used to remove callbacks after error occurs in send().
func (c *Client) removeCallbacks(callbacks map[string]dnode.Path) {
	for sid := range callbacks {
		// We don't check for error because we have created
		// the callbacks map in the send function above.
		// It does not come from remote, so cannot contain errors.
		id, _ := strconv.ParseUint(sid, 10, 64)
		c.scrubber.RemoveCallback(id)
	}
}

// sendCallbackID send the callback number to be deleted after response is received.
func sendCallbackID(callbacks map[string]dnode.Path, ch chan<- uint64) {
	// TODO fix finding of responseCallback in dnode message when removing callback
	for id, path := range callbacks {
		if len(path) != 2 {
			continue
		}
		p0, ok := path[0].(string)
		if !ok {
			continue
		}
		p1, ok := path[1].(string)
		if !ok {
			continue
		}
		if p0 != "0" || p1 != "responseCallback" {
			continue
		}
		i, _ := strconv.ParseUint(id, 10, 64)
		ch <- i
		return
	}
	close(ch)
}

// makeResponseCallback prepares and returns a callback function sent to the server.
// The caller of the Tell() is blocked until the server calls this callback function.
// Sets theResponse and notifies the caller by sending to done channel.
func (c *Client) makeResponseCallback(doneChan chan *response, removeCallback <-chan uint64, method string, args []interface{}) dnode.Function {
	return dnode.Callback(func(arguments *dnode.Partial) {
		// Single argument of response callback.
		var resp struct {
			Result *dnode.Partial `json:"result"`
			Err    *Error         `json:"error"`
		}

		// Notify that the callback is finished.
		defer func() {
			if resp.Err != nil {
				c.LocalKite.Log.Debug("Error received from kite: %q method: %q args: %#v err: %s", c.Kite.Name, method, args, resp.Err.Error())
				doneChan <- &response{resp.Result, resp.Err}
			} else {
				doneChan <- &response{resp.Result, nil}
			}
		}()

		// Remove the callback function from the map so we do not
		// consume memory for unused callbacks.
		if id, ok := <-removeCallback; ok {
			c.scrubber.RemoveCallback(id)
		}

		// We must only get one argument for response callback.
		arg, err := arguments.SliceOfLength(1)
		if err != nil {
			resp.Err = &Error{Type: "invalidResponse", Message: err.Error()}
			return
		}

		// Unmarshal callback response argument.
		err = arg[0].Unmarshal(&resp)
		if err != nil {
			resp.Err = &Error{Type: "invalidResponse", Message: err.Error()}
			return
		}

		// At least result or error must be sent.
		keys := make(map[string]interface{})
		err = arg[0].Unmarshal(&keys)
		_, ok1 := keys["result"]
		_, ok2 := keys["error"]
		if !ok1 && !ok2 {
			resp.Err = &Error{
				Type:    "invalidResponse",
				Message: "Server has sent invalid response arguments",
			}
			return
		}
	})
}

// onError is called when an error happened in a method handler.
func onError(err error) {
	// TODO do not marshal options again here
	switch e := err.(type) {
	case dnode.MethodNotFoundError: // Tell the requester "method is not found".
		args, err2 := e.Args.Slice()
		if err2 != nil {
			return
		}

		if len(args) < 1 {
			return
		}

		var options callOptions
		if err := args[0].Unmarshal(&options); err != nil {
			return
		}

		if options.ResponseCallback.Caller != nil {
			response := Response{
				Result: nil,
				Error: &Error{
					Type:    "methodNotFound",
					Message: err.Error(),
				},
			}
			options.ResponseCallback.Call(response)
		}
	}
}
