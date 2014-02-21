package rpc

import (
	"errors"
	"fmt"
	"github.com/koding/kite/dnode"
	"net/url"
	"os"
	"sync"
	"time"

	"code.google.com/p/go.net/websocket"
)

const redialDurationStart = 1 * time.Second
const redialDurationMax = 60 * time.Second

// Dial is a helper for creating a Client for just calling methods on the server.
// Do not use it if you want to handle methods on client side. Instead create a
// new Client, register your methods on Client.Dnode then call Client.Dial().
func Dial(url string, reconnect bool) (*Client, error) {
	c := NewClient()
	c.Reconnect = reconnect

	err := c.Dial(url)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// Client is a dnode RPC client.
type Client struct {
	// Websocket connection
	Conn *websocket.Conn

	// Websocket connection options.
	Config *websocket.Config

	// Dnode message processor.
	dnode *dnode.Dnode

	// A space for saving/reading extra properties about this client.
	properties map[string]interface{}

	// Should we reconnect if disconnected?
	Reconnect bool

	// Time to wait before redial connection.
	redialDuration time.Duration

	// on connect/disconnect handlers are invoked after every
	// connect/disconnect.
	onConnectHandlers    []func()
	onDisconnectHandlers []func()

	// For protecting access over OnConnect and OnDisconnect handlers.
	m sync.RWMutex
}

// NewClient returns a pointer to new Client.
// You need to call Dial() before interacting with the Server.
func NewClient() *Client {
	// Must send an "Origin" header. Does not checked on server.
	origin, _ := url.Parse("")

	config := &websocket.Config{
		Version: websocket.ProtocolVersionHybi13,
		Origin:  origin,
		// Location will be set when dialing.
	}

	c := &Client{
		properties:     make(map[string]interface{}),
		redialDuration: redialDurationStart,
		Config:         config,
	}

	c.dnode = dnode.New(c)
	return c
}

func (c *Client) SetWrappers(wrapMethodArgs, wrapCallbackArgs dnode.Wrapper, runMethod, runCallback dnode.Runner, onError func(error)) {
	c.dnode.WrapMethodArgs = wrapMethodArgs
	c.dnode.WrapCallbackArgs = wrapCallbackArgs
	c.dnode.RunMethod = runMethod
	c.dnode.RunCallback = runCallback
	c.dnode.OnError = onError
}

// Dial connects to the dnode server on "url" and starts a goroutine
// that processes incoming messages.
//
// Do not forget to register your handlers on Client.Dnode
// before calling Dial() to prevent race conditions.
func (c *Client) Dial(serverURL string) error {
	var err error

	if c.Config.Location, err = url.Parse(serverURL); err != nil {
		return err
	}

	if err = c.dial(); err != nil {
		return err
	}

	go c.run()

	return nil
}

// dial makes a single Dial() and run onConnectHandlers if connects.
func (c *Client) dial() error {
	ws, err := websocket.DialConfig(c.Config)
	if err != nil {
		return err
	}

	// We are connected
	c.Conn = ws

	// Reset the wait time.
	c.redialDuration = redialDurationStart

	// Must be run in a goroutine because a handler may wait a response from
	// server.
	go c.callOnConnectHandlers()

	return nil
}

// DialForever connects to the server in background.
// If the connection drops, it reconnects again.
func (c *Client) DialForever(serverURL string) (err error) {
	c.Reconnect = true

	if c.Config.Location, err = url.Parse(serverURL); err != nil {
		return
	}

	go c.dialForever()

	return
}

func (c *Client) dialForever() {
	for c.dial() != nil {
		if !c.Reconnect {
			return
		}

		c.sleep()
	}
	go c.run()
}

// run consumes incoming dnode messages. Reconnects if necessary.
func (c *Client) run() (err error) {
	for {
	running:
		err = c.dnode.Run()
		c.callOnDisconnectHandlers()
	dialAgain:
		if !c.Reconnect {
			break
		}

		err = c.dial()
		if err != nil {
			c.sleep()
			goto dialAgain
		}

		goto running
	}

	return err
}

// sleep is used to wait for a while between dial retries.
// Each time it is called the redialDuration is incremented.
func (c *Client) sleep() {
	time.Sleep(c.redialDuration)

	c.redialDuration *= 2
	if c.redialDuration > redialDurationMax {
		c.redialDuration = redialDurationMax
	}
}

// Close closes the underlying websocket connection.
func (c *Client) Close() {
	c.Reconnect = false
	c.Conn.Close()
}

func (c *Client) Send(msg []byte) error {
	if os.Getenv("DNODE_PRINT_SEND") != "" {
		fmt.Fprintf(os.Stderr, "\nSending: %s\n", string(msg))
	}

	if c.Conn == nil {
		return errors.New("Not connected")
	}

	return websocket.Message.Send(c.Conn, string(msg))
}

func (c *Client) Receive() ([]byte, error) {
	// println("Receiving...")
	var msg []byte
	err := websocket.Message.Receive(c.Conn, &msg)

	if os.Getenv("DNODE_PRINT_RECV") != "" {
		fmt.Fprintf(os.Stderr, "\nReceived: %s\n", string(msg))
	}

	return msg, err
}

func (c *Client) RemoveCallback(id uint64) {
	c.dnode.RemoveCallback(id)
}

// RemoteAddr returns the host:port as string if server connection.
func (c *Client) RemoteAddr() string {
	if c.Conn.IsServerConn() {
		return c.Conn.Request().RemoteAddr
	}
	return ""
}

func (c *Client) Properties() map[string]interface{} {
	return c.properties
}

// Call calls a method with args on the dnode server.
func (c *Client) Call(method string, args ...interface{}) (map[string]dnode.Path, error) {
	return c.dnode.Call(method, args...)
}

// OnConnect registers a function to run on client connect.
func (c *Client) OnConnect(handler func()) {
	c.m.Lock()
	c.onConnectHandlers = append(c.onConnectHandlers, handler)
	c.m.Unlock()
}

// OnDisconnect registers a function to run on client disconnect.
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
