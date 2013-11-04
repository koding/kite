package kite

import (
	"code.google.com/p/go.net/websocket"
	"sync"
)

type client struct {
	Addr     string
	Username string
	Conn     *websocket.Conn
	// functions to be called when client disconnects from kite
	onDisconnect []func()
}

type clients struct {
	// key is address of the connected client
	clients map[string]*client

	// key is the username of the connected client after authentication
	addresses map[string][]string

	sync.Mutex //protects the maps above
}

func NewClient() *client {
	return &client{
		Addr:         "",
		Username:     "",
		Conn:         nil,
		onDisconnect: make([]func(), 0),
	}
}

func NewClients() *clients {
	return &clients{
		clients:   make(map[string]*client),
		addresses: make(map[string][]string),
	}
}

func (c *clients) AddClient(addr string, client *client) {
	c.Lock()
	defer c.Unlock()

	c.clients[addr] = client
}

func (c *clients) GetClient(addr string) *client {
	c.Lock()
	defer c.Unlock()

	res, ok := c.clients[addr]
	if !ok {
		return nil
	}

	return res
}

func (c *clients) AddAddresses(user, addr string) {
	c.Lock()
	defer c.Unlock()

	addrs, ok := c.addresses[user]
	if !ok {
		newAddrs := []string{addr}
		c.addresses[user] = newAddrs
	} else {
		addrs = append(addrs, addr)
		c.addresses[user] = addrs
	}

}

func (c *clients) GetAddresses(username string) []string {
	c.Lock()
	defer c.Unlock()

	addrs, ok := c.addresses[username]
	if !ok {
		return nil
	}

	return addrs
}

func (c *clients) RemoveClient(addr string) {
	c.Lock()
	defer c.Unlock()

	delete(c.clients, addr)
}

func (c *clients) RemoveAddresses(username, clientAddr string) {
	c.Lock()
	defer c.Unlock()

	addrs, ok := c.addresses[username]
	if !ok {
		return
	}

	// means last connected client, remove it from the map and return
	if len(addrs) == 1 {
		delete(c.addresses, username)
		return
	}

	// remove the addr from the addresses by creating a new map without it
	newAddrs := make([]string, 0)
	for _, addr := range addrs {
		if addr == clientAddr {
			continue // don't add it to our new map
		}

		newAddrs = append(newAddrs, addr)
	}

	c.addresses[username] = newAddrs
}

func (c *clients) Size() int {
	c.Lock()
	defer c.Unlock()

	return len(c.clients)
}

func (c *clients) List() []*client {
	c.Lock()
	defer c.Unlock()
	list := make([]*client, len(c.clients))

	if len(c.clients) == 0 {
		return list
	}

	i := 0
	for _, client := range c.clients {
		list[i] = client
		i++
	}

	return list
}
