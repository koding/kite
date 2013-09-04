package kite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"kite/protocol"
	"log"
	"net"
	"net/http"

	"os/user"
	"strings"
)

// Listen returns a Listener that listens on the first available port on the
// first available non-loopback IPv4 network interface.
func listenExternal() (net.Listener, error) {
	ip, err := externalIP()
	if err != nil {
		return nil, fmt.Errorf("could not find active non-loopback address: %v", err)
	}
	return net.Listen("tcp4", ip+":0") // picks up a random port if zero
}

// returns on of the local network interfaces IP
func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}

func getKey(key string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	var keyfile string
	switch key {
	case "public":
		keyfile = usr.HomeDir + "/.kd/koding.key.pub"
	case "private":
		keyfile = usr.HomeDir + "/.kd/koding.key"
	default:
		return "", fmt.Errorf("key is not recognized '%s'\n", key)
	}

	file, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(file)), nil
}

// return o.LocalIP back if assigned, otherwise it gets a local IP from on
// of the local network interfaces
func getLocalIP(ip string) string {
	// already assigned manually
	if ip != "" {
		return ip
	}

	// if no assigned manually, then pick up one from the internal interfaces
	var err error
	ip, err = externalIP()
	if err != nil {
		//	There is no ip assigned manually neither can we find any
		//	external IP, therefore abort, because kite can't work in this
		//	state.
		log.Fatalln(err)
	}
	return ip
}

// returns o.PublicIP back if assigned, otherwise it gets a public IP from
// a public service (like icanhazip.com)
func getPublicIP(ip string) string {
	// already assigned manually
	if ip != "" {
		return ip
	}

	resp, err := http.Get("http://icanhazip.com")
	if err != nil {
		return ""
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return ""
	}

	// validate incoming data, it should be a valid IP
	netIP := net.ParseIP(strings.TrimSpace(string(body)))
	if netIP == nil {
		return ""
	}

	return netIP.To4().String()
}

func readOptions(configfile string) (*protocol.Options, error) {
	file, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, err
	}

	options := &protocol.Options{}
	err = json.Unmarshal(file, &options)
	if err != nil {
		return nil, err
	}

	return options, nil
}

func debug(args ...interface{}) {
	if protocol.DEBUG_ENABLED {
		fmt.Println(args...)
	}
}
