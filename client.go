package kite

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/sockjsclient"

	"github.com/cenkalti/backoff"
	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

var forever = backoff.NewExponentialBackOff()

func init() {
	forever.MaxElapsedTime = 365 * 24 * time.Hour // 1 year
}

func nopSetSession(sockjs.Session) {}

// Client is the client for communicating with another Kite.
// It has Tell() and Go() methods for calling methods sync/async way.
type Client struct {
	protocol.Kite // remote kite information

	// LocalKite references to the kite which owns the client
	// connection.
	LocalKite *Kite

	// Auth is a credential used to authenticate with a remote kite.
	//
	// Required if remote kite requires authentication.
	Auth *Auth

	// Reconnect says whether we should reconnect with a new
	// session when an old one got invalidated or the connection
	// broke.
	Reconnect bool

	// URL specifies the SockJS URL of the remote kite.
	URL string

	// Config is used when setting up client connection to
	// the remote kite.
	//
	// If Config is nil, LocalKite.Config is used instead.
	Config *config.Config

	// Concurrent specified whether we should process incoming messages concurrently.
	//
	// Defaults to true.
	Concurrent bool

	// ConcurrentCallbacks, when true, makes execution of callbacks in
	// incoming messages concurrent. This may result in a callback
	// received in an earlier message to be executed after a callback
	// from a new meesage - no order is guaranteed, it's up to Go scheduler.
	//
	// By default this field is false to be backward-compatible with
	// go1.4 scheduling behaviour.
	ConcurrentCallbacks bool

	// ClientFunc is called each time new sockjs.Session is established.
	// The session will use returned *http.Client for HTTP round trips
	// for XHR transport.
	//
	// If ClientFunc is nil, sockjs.Session will use default, internal
	// *http.Client value.
	//
	// Deprecated: Set Config.XHR of the local kite, that
	// owns this connection, insead.
	ClientFunc func(*sockjsclient.DialOptions) *http.Client

	// ReadBufferSize is the input buffer size. By default it's 4096.
	//
	// Deprecated: Set Config.Websocket.ReadBufferSize of the local kite,
	// that owns this connection, instead.
	ReadBufferSize int

	// WriteBufferSize is the output buffer size. By default it's 4096.
	//
	// Deprecated: Set Config.Websocket.WriteBufferSize of the local kite,
	// that owns this connection, instead.
	WriteBufferSize int

	muProt sync.Mutex // protects protocol.Kite access

	// To signal waiters of Go() on disconnect.
	disconnect   chan struct{}
	disconnectMu sync.Mutex // protects disconnect chan

	// authMu protects Auth field.
	authMu sync.Mutex

	// To signal about the close
	closeChan chan struct{}

	// closeRenewer is used to stop renewing tokens when client
	// is closed but was not dialed
	closeRenewer chan struct{}

	// interrupt is used to signalise readloop that
	// session was interrupted.
	interrupt chan error

	// To syncronize the consumers
	wg sync.WaitGroup

	// SockJS session
	// TODO: replace this with a proper interface to support multiple
	// transport/protocols
	session sockjs.Session
	send    chan *message

	// muReconnect protects Reconnect
	muReconnect sync.Mutex

	// closed is to ensure Close is idempotent
	closed int32

	// dnode scrubber for saving callbacks sent to remote.
	scrubber *dnode.Scrubber

	// Time to wait before redial connection.
	redialBackOff backoff.ExponentialBackOff

	// on connect/disconnect handlers are invoked after every
	// connect/disconnect.
	onConnectHandlers     []func()
	onDisconnectHandlers  []func()
	onTokenExpireHandlers []func()
	onTokenRenewHandlers  []func(string)

	testHookSetSession func(sockjs.Session)

	// For protecting access over OnConnect and OnDisconnect handlers.
	m sync.RWMutex

	firstRequestHandlersNotified sync.Once
}

// message carries an encoded payload sent over connected session.
type message struct {
	p    []byte
	errC chan<- error
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
	c := &Client{
		LocalKite:          k,
		URL:                remoteURL,
		disconnect:         make(chan struct{}),
		closeChan:          make(chan struct{}),
		redialBackOff:      *forever,
		scrubber:           dnode.NewScrubber(),
		testHookSetSession: nopSetSession,
		Concurrent:         true,
		send:               make(chan *message),
		interrupt:          make(chan error, 1),
	}

	k.OnRegister(c.updateAuth)

	return c
}

func (c *Client) SetUsername(username string) {
	c.muProt.Lock()
	c.Kite.Username = username
	c.muProt.Unlock()
}

// Dial connects to the remote Kite. Returns error if it can't.
func (c *Client) Dial() (err error) {
	// zero means no timeout
	return c.DialTimeout(0)
}

// DialTimeout acts like Dial but takes a timeout.
func (c *Client) DialTimeout(timeout time.Duration) error {
	err := c.dial(timeout)

	c.LocalKite.Log.Debug("Dialing '%s' kite: %s (error: %v)", c.Kite.Name, c.URL, err)

	if err != nil {
		return err
	}

	go c.run()

	return nil
}

// Dial connects to the remote Kite. If it can't connect, it retries
// indefinitely. It returns a channel to check if it's connected or not.
func (c *Client) DialForever() (connected chan bool, err error) {
	c.Reconnect = true
	connected = make(chan bool, 1) // This will be closed on first connection.
	go c.dialForever(connected)
	return
}

func (c *Client) updateAuth(reg *protocol.RegisterResult) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if c.Auth == nil {
		return
	}

	if c.Auth.Type == "kiteKey" && reg.KiteKey != "" {
		c.Auth.Key = reg.KiteKey
	}
}

func (c *Client) authCopy() *Auth {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if c.Auth == nil {
		return nil
	}

	authCopy := *c.Auth
	return &authCopy
}

func (c *Client) dial(timeout time.Duration) (err error) {
	transport := c.config().Transport

	c.LocalKite.Log.Debug("Client transport is set to '%s'", transport)

	var session sockjs.Session

	switch transport {
	case config.WebSocket:
		session, err = sockjsclient.DialWebsocket(c.URL, c.config())
	case config.XHRPolling:
		session, err = sockjsclient.DialXHR(c.URL, c.config())
	case config.Auto:
		session, err = sockjsclient.DialWebsocket(c.URL, c.config())
		if err == websocket.ErrBadHandshake {
			// In cases when kite server is behind a proxy that do
			// not support websocket connections, fall back to XHR.
			session, err = sockjsclient.DialXHR(c.URL, c.config())
		}
	default:
		return fmt.Errorf("Connection transport is not known '%v'", transport)
	}

	if err != nil {
		return err
	}

	c.setSession(session)
	c.wg.Add(1)
	go c.sendHub()

	// Reset the wait time.
	c.redialBackOff.Reset()

	// Must be run in a goroutine because a handler may wait a response from
	// server.
	go c.callOnConnectHandlers()

	return nil
}

func (c *Client) dialForever(connectNotifyChan chan bool) {
	dial := func() error {
		if !c.reconnect() {
			return nil
		}

		c.LocalKite.Log.Info("Dialing '%s' kite: %s", c.Kite.Name, c.URL)

		if err := c.dial(0); err != nil {
			c.LocalKite.Log.Warning("Dialing '%s' kite error: %s: %v", c.Kite.Name, c.URL, err)

			return err
		}

		return nil
	}

	backoff.Retry(dial, &c.redialBackOff) // this will retry dial forever

	if connectNotifyChan != nil {
		close(connectNotifyChan)
	}

	go c.run()
}

func (c *Client) RemoteAddr() string {
	session := c.getSession()
	if session == nil {
		return ""
	}

	websocketsession, ok := session.(*sockjsclient.WebsocketSession)
	if !ok {
		return ""
	}

	return websocketsession.RemoteAddr()
}

// run consumes incoming dnode messages. Reconnects if necessary.
func (c *Client) run() {
	err := c.readLoop()
	if err != nil {
		c.LocalKite.Log.Debug("readloop err: %s", err)
	}

	// falls here when connection disconnects
	c.callOnDisconnectHandlers()

	// let others know that the client has disconnected
	c.disconnectMu.Lock()
	if c.disconnect != nil {
		close(c.disconnect)
		c.disconnect = nil
	}
	c.disconnectMu.Unlock()

	if c.reconnect() {
		// we override it so it doesn't get selected next time. Because we are
		// redialing, so after redial if a new method is called, the disconnect
		// channel is being read and the local "disconnect" message will be the
		// final response. This shouldn't be happen for redials.
		c.disconnectMu.Lock()
		c.disconnect = make(chan struct{}, 1)
		c.disconnectMu.Unlock()
		go c.dialForever(nil)
	}
}

func (c *Client) reconnect() bool {
	c.muReconnect.Lock()
	defer c.muReconnect.Unlock()

	return c.Reconnect
}

// readLoop reads a message from websocket and processes it.
func (c *Client) readLoop() error {
	for {
		p, err := c.receiveData()

		c.LocalKite.Log.Debug("readloop received: %s %v", p, err)

		if err != nil {
			return err
		}

		msg, fn, err := c.processMessage(p)
		if err != nil {
			if _, ok := err.(dnode.CallbackNotFoundError); !ok {
				c.LocalKite.Log.Warning("error processing message err: %s message: %s", err, msg)
			}
		}

		switch v := fn.(type) {
		case *Method: // invoke method
			if c.Concurrent {
				go c.runMethod(v, msg.Arguments)
			} else {
				c.runMethod(v, msg.Arguments)
			}
		case func(*dnode.Partial): // invoke callback
			if c.Concurrent && c.ConcurrentCallbacks {
				go c.runCallback(v, msg.Arguments)
			} else {
				c.runCallback(v, msg.Arguments)
			}
		}
	}
}

// receiveData reads a message from session.
func (c *Client) receiveData() ([]byte, error) {
	type recv struct {
		msg []byte
		err error
	}

	session := c.getSession()
	if session == nil {
		return nil, errors.New("not connected")
	}

	done := make(chan recv, 1)

	go func() {
		msg, err := session.Recv()
		done <- recv{[]byte(msg), err}
	}()

	select {
	case r := <-done:
		return r.msg, r.err
	case err := <-c.interrupt:
		return nil, err
	}
}

// processMessage processes a single message and calls a handler or callback.
func (c *Client) processMessage(data []byte) (msg *dnode.Message, fn interface{}, err error) {
	// Call error handler.
	defer func() {
		if err != nil {
			onError(err)
		}
	}()

	msg = &dnode.Message{}

	if err = json.Unmarshal(data, &msg); err != nil {
		return nil, nil, err
	}

	sender := func(id uint64, args []interface{}) error {
		// do not name the error variable to "err" here, it's a trap for
		// shadowing variables
		_, _, e := c.marshalAndSend(id, args)
		return e
	}

	// Replace function placeholders with real functions.
	if err := dnode.ParseCallbacks(msg, sender); err != nil {
		return nil, nil, err
	}

	// Find the handler function. Method may be string or integer.
	switch method := msg.Method.(type) {
	case float64:
		id := uint64(method)
		callback := c.scrubber.GetCallback(id)
		if callback == nil {
			err = dnode.CallbackNotFoundError{
				ID:   id,
				Args: msg.Arguments,
			}
			return nil, nil, err
		}

		return msg, callback, nil
	case string:
		m, ok := c.LocalKite.handlers[method]
		if !ok {
			err = dnode.MethodNotFoundError{
				Method: method,
				Args:   msg.Arguments,
			}
			return nil, nil, err
		}

		return msg, m, nil
	default:
		return nil, nil, fmt.Errorf("Method is not string or integer: %+v (%T)", msg.Method, msg.Method)
	}
}

func (c *Client) Close() {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return // TODO: ErrAlreadyClosed
	}

	c.muReconnect.Lock()
	// TODO(rjeczalik): add another internal field for controlling redials
	// instead of mutating public field
	c.Reconnect = false
	c.muReconnect.Unlock()

	close(c.closeChan)

	if c.closeRenewer != nil {
		select {
		case c.closeRenewer <- struct{}{}:
		default:
		}
	}

	// wait for consumers to finish buffered messages
	c.wg.Wait()

	if session := c.getSession(); session != nil {
		session.Close(3000, "Go away!")
	}
}

// sendhub sends the msg received from the send channel to the remote client
func (c *Client) sendHub() {
	defer c.wg.Done()

	for {
		select {
		case msg := <-c.send:
			c.LocalKite.Log.Debug("sending: %s", msg)
			session := c.getSession()
			if session == nil {
				c.LocalKite.Log.Error("not connected")
				continue
			}

			err := session.Send(string(msg.p))
			if err != nil {
				if msg.errC != nil {
					msg.errC <- err
				}

				if sockjsclient.IsSessionClosed(err) {
					// The readloop may already be interrupted, thus the non-blocking send.
					select {
					case c.interrupt <- err:
					default:
					}

					c.LocalKite.Log.Error("error sending to %s: %s", session.ID(), err)
					return
				}
			}
		case <-c.closeChan:
			c.LocalKite.Log.Debug("Send hub is closed")
			return
		}
	}
}

// OnConnect adds a callback which is called when client connects
// to a remote kite.
func (c *Client) OnConnect(handler func()) {
	c.m.Lock()
	c.onConnectHandlers = append(c.onConnectHandlers, handler)
	c.m.Unlock()
}

// OnDisconnect adds a callback which is called when client disconnects
// from a remote kite.
func (c *Client) OnDisconnect(handler func()) {
	c.m.Lock()
	c.onDisconnectHandlers = append(c.onDisconnectHandlers, handler)
	c.m.Unlock()
}

// OnTokenExpire adds a callback which is called when client receives
// token-is-expired error from a remote kite.
func (c *Client) OnTokenExpire(handler func()) {
	c.m.Lock()
	c.onTokenExpireHandlers = append(c.onTokenExpireHandlers, handler)
	c.m.Unlock()
}

// OnTokenRenew adds a callback which is called when client successfully
// renews its token.
func (c *Client) OnTokenRenew(handler func(token string)) {
	c.m.Lock()
	c.onTokenRenewHandlers = append(c.onTokenRenewHandlers, handler)
	c.m.Unlock()
}

// callOnConnectHandlers runs the registered connect handlers.
func (c *Client) callOnConnectHandlers() {
	c.m.RLock()
	defer c.m.RUnlock()

	for _, handler := range c.onConnectHandlers {
		func() {
			defer nopRecover()
			handler()
		}()
	}
}

// callOnDisconnectHandlers runs the registered disconnect handlers.
func (c *Client) callOnDisconnectHandlers() {
	c.m.RLock()
	defer c.m.RUnlock()

	for _, handler := range c.onDisconnectHandlers {
		func() {
			defer nopRecover()
			handler()
		}()
	}
}

// callOnTokenExpireHandlers calls registered functions when an error
// from remote kite is received that token used is expired.
func (c *Client) callOnTokenExpireHandlers() {
	c.m.RLock()
	defer c.m.RUnlock()

	for _, handler := range c.onTokenExpireHandlers {
		func() {
			defer nopRecover()
			handler()
		}()
	}
}

// callOnTokenRenewHandlers calls all registered functions when
// we successfully obtain new token from kontrol.
func (c *Client) callOnTokenRenewHandlers(token string) {
	c.m.RLock()
	defer c.m.RUnlock()

	for _, handler := range c.onTokenRenewHandlers {
		func() {
			defer nopRecover()
			handler(token)
		}()
	}
}

func (c *Client) wrapMethodArgs(args []interface{}, responseCallback dnode.Function) []interface{} {
	options := callOptionsOut{
		WithArgs: args,
		callOptions: callOptions{
			Kite:             *c.LocalKite.Kite(),
			Auth:             c.authCopy(),
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

	callbacks, errC, err := c.marshalAndSend(method, args)
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
		c.disconnectMu.Lock()
		defer c.disconnectMu.Unlock()

		select {
		case resp := <-doneChan:
			if e, ok := resp.Err.(*Error); ok {
				if e.Type == "authenticationError" && strings.Contains(e.Message, "token is expired") {
					c.callOnTokenExpireHandlers()
				}
			}

			responseChan <- resp
		case <-c.disconnect:
			responseChan <- &response{
				nil,
				&Error{
					Type:    "disconnect",
					Message: "Remote kite has disconnected",
				},
			}
		case err := <-errC:
			if err != nil {
				responseChan <- &response{
					nil,
					&Error{
						Type:    "sendError",
						Message: err.Error(),
					},
				}
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
func (c *Client) marshalAndSend(method interface{}, arguments []interface{}) (callbacks map[string]dnode.Path, errC <-chan error, err error) {
	// scrub trough the arguments and save any callbacks.
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
		return nil, nil, err
	}

	msg := dnode.Message{
		Method:    method,
		Arguments: &dnode.Partial{Raw: rawArgs},
		Callbacks: callbacks,
	}

	p, err := json.Marshal(msg)
	if err != nil {
		return nil, nil, err
	}

	select {
	case <-c.closeChan:
		return nil, nil, errors.New("can't send, client is closed")
	default:
		if c.getSession() == nil {
			return nil, nil, errors.New("can't send, session is not established yet")
		}

		errC := make(chan error, 1)

		c.send <- &message{
			p:    p,
			errC: errC,
		}

		return callbacks, errC, nil
	}
}

func (c *Client) getSession() sockjs.Session {
	c.m.RLock()
	defer c.m.RUnlock()

	return c.session
}

func (c *Client) setSession(session sockjs.Session) {
	c.testHookSetSession(session)

	c.m.Lock()
	c.session = session
	c.m.Unlock()
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

func (c *Client) config() *config.Config {
	if c.Config != nil {
		return c.Config
	}
	return c.LocalKite.Config
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
