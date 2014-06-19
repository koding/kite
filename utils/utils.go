package utils

import (
	"net"
	"strconv"
)

// RandomPort() returns a random port to be used with net.Listen(). It's an
// helper function to register to kontrol before binding to a port. Note that
// this racy, there is a possibility that someoe binds to the port during the
// time you get the port and someone else finds it. Therefore use in caution.
func RandomPort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(port)
}
