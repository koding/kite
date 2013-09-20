package kite

import (
	"code.google.com/p/go.net/websocket"
	"sync"
)

type client struct {
	Addr     string
	Token    string
	Username string
	Conn     *websocket.Conn
}

type clients struct {
	m map[string]*client
	sync.Mutex
}

func NewClients() *clients {
	return &clients{
		m: make(map[string]*client),
	}
}

func (c *clients) Add(client *client) {
	c.Lock()
	defer c.Unlock()

	ok := c.validate(client)
	if !ok {
		return
	}

	c.m[client.Addr] = client
}

func (c *clients) Get(client *client) *client {
	c.Lock()
	defer c.Unlock()

	ok := c.validate(client)
	if !ok {
		return nil
	}

	res, ok := c.m[client.Addr]
	if !ok {
		return nil
	}

	return res
}

func (c *clients) Remove(client *client) {
	c.Lock()
	defer c.Unlock()

	ok := c.validate(client)
	if !ok {
		return
	}

	delete(c.m, client.Addr)
}

func (c *clients) Size() int {
	c.Lock()
	defer c.Unlock()
	return len(c.m)
}

func (c *clients) List() []*client {
	c.Lock()
	defer c.Unlock()
	list := make([]*client, len(c.m))

	if len(c.m) == 0 {
		return list
	}

	i := 0
	for _, client := range c.m {
		list[i] = client
		i++
	}
	return list

}

func (c *clients) validate(client *client) bool {
	if client == nil {
		return false
	}

	if client.Addr == "" {
		return false
	}

	return true
}
