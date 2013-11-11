package rpc

import (
	"code.google.com/p/go.net/websocket"
	"koding/newkite/dnode"
)

type Client struct {
	ws    *websocket.Conn
	dnode *dnode.Dnode
}

func Dial(url string) (*Client, error) {
	ws, err := websocket.Dial(url, "", "http://localhost")
	if err != nil {
		return nil, err
	}

	c := newClient(ws)
	go c.run()
	return c, nil
}

func (c *Client) Close() {
	c.ws.Close()
}

func (c *Client) Call(method string, args ...interface{}) {
	c.dnode.Call(method, args...)
}

func newClient(ws *websocket.Conn) *Client {
	tr := &wsTransport{ws}
	return &Client{
		ws:    ws,
		dnode: dnode.New(tr),
	}
}

func (c *Client) run() {
	c.dnode.Run()
}

type wsTransport struct {
	ws *websocket.Conn
}

func (t *wsTransport) Send(msg []byte) error {
	return websocket.Message.Send(t.ws, string(msg))
}

func (t *wsTransport) Receive() ([]byte, error) {
	var msg []byte
	err := websocket.Message.Receive(t.ws, &msg)
	return msg, err
}
