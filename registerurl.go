package kite

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
)

const publicEcho = "http://echoip.com"

// RegisterURL returns a URL that is either local or public. It's an helper
// method to get a Registration URL that can be passed to Kontrol (via the
// methods Register(), RegisterToProxy(), etc.) It needs to be called after all
// configurations are done (like TLS, Port,etc.). If local is true a local IP
// is used, otherwise a public IP is being used.
func (k *Kite) RegisterURL(local bool) *url.URL {
	var ip net.IP
	var err error

	if local {
		ip, err = localIP()
		if err != nil {
			return nil
		}
	} else {
		ip, err = publicIP()
		if err != nil {
			return nil
		}
	}

	scheme := "http"
	if k.TLSConfig != nil {
		scheme = "https"
	}

	return &url.URL{
		Scheme: scheme,
		Host:   ip.String() + ":" + strconv.Itoa(k.Config.Port),
		Path:   "/" + k.name + "-" + k.version + "/kite",
	}
}

// localIp returns a local IP from one of the local interfaces.
func localIP() (net.IP, error) {
	tt, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, t := range tt {
		aa, err := t.Addrs()
		if err != nil {
			return nil, err
		}
		for _, a := range aa {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()

			if v4 == nil || v4[0] == 127 { // loopback address
				continue
			}
			return v4, nil
		}
	}

	return nil, errors.New("cannot find local IP address")
}

// publicIP returns an IP that is supposed to be Public.
func publicIP() (net.IP, error) {
	resp, err := http.Get(publicEcho)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// The ip address is 16 chars long, we read more
	// to account for excessive whitespace.
	p, err := ioutil.ReadAll(io.LimitReader(resp.Body, 24))
	if err != nil {
		return nil, err
	}

	n := net.ParseIP(string(bytes.TrimSpace(p)))
	if n == nil {
		return nil, fmt.Errorf("cannot parse ip %s", p)
	}

	return n, nil
}
