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
	c.dnode.Close()
	c.ws.Close()
}

func (c *Client) Call(method string, args ...interface{}) {
	c.dnode.Call(method, args...)
}

func newClient(ws *websocket.Conn) *Client {
	return &Client{
		ws:    ws,
		dnode: dnode.New(),
	}
}

func (c *Client) run() {
	go c.dnode.Run()
	go c.writer()
	c.reader()
}

func (c *Client) reader() {
	for {
		var msg dnode.Message
		if websocket.JSON.Receive(c.ws, &msg) != nil {
			break
		}
		c.dnode.ReceiveChan <- msg
	}
}

func (c *Client) writer() {
	for msg := range c.dnode.SendChan {
		if websocket.JSON.Send(c.ws, msg) != nil {
			break
		}
	}
}
