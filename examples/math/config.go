package math

import (
	"flag"
	"net"
	"net/url"
	"strconv"
)

type Request struct {
	Number int
	Name   string
}

var Host = &Config{
	URL: &url.URL{
		Scheme: "http",
		Path:   "/kite",
		Host:   "127.0.0.1:3636",
	},
}

func init() {
	flag.Var(Host, "addr", "Network address of the kite server.")
}

type Config struct {
	URL *url.URL
}

func (c *Config) IP() string {
	ip, _, _ := net.SplitHostPort(c.URL.Host)
	return ip
}

func (c *Config) Port() int {
	_, s, _ := net.SplitHostPort(c.URL.Host)
	port, _ := strconv.Atoi(s)
	return port
}

func (c *Config) Set(s string) error {
	if _, _, err := net.SplitHostPort(s); err != nil {
		return err
	}
	c.URL.Host = s
	return nil
}

func (c *Config) String() string {
	return c.URL.Host
}
