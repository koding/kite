package kite

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"math"
	"net"
	"net/http"
	"net/rpc"
	"time"
)

// Remote encapsulates kites of specified type of specified user.
type Remote struct {
	Username string
	Kitename string
	Kites    []*RemoteKite
}

// RemoteKite is the structure representing other connected kites.
type RemoteKite struct {
	protocol.KiteWithToken
	Client *rpc.Client
}

// Remote is used to create a new remote struct that is used for remote
// kite-to-kite calls.
func (k *Kite) Remote(username, kitename string) (*Remote, error) {
	remoteKites, err := k.requestKites(username, kitename)
	if err != nil {
		return nil, fmt.Errorf("Cannot get remote kites: %s", err.Error())
	}
	if len(remoteKites) == 0 {
		return nil, fmt.Errorf("No remote kites available for %s/%s", username, kitename)
	}

	return &Remote{
		Username: username,
		Kitename: kitename,
		Kites:    remoteKites,
	}, nil
}

func (k *Kite) requestKites(username, kitename string) ([]*RemoteKite, error) {
	m := protocol.KiteToKontrolRequest{
		Kite:      k.Kite,
		KodingKey: k.KodingKey,
		Method:    protocol.GetKites,
		Args: map[string]interface{}{
			"username": username,
			"kitename": kitename,
		},
	}

	msg, err := json.Marshal(&m)
	if err != nil {
		slog.Println("requestKites marshall err 1", err)
		return nil, err
	}

	slog.Println("sending requesting message...")
	result, err := k.kontrolClient.Request(msg)
	if err != nil {
		return nil, err
	}

	var kitesResp protocol.GetKitesResponse
	err = json.Unmarshal(result, &kitesResp)
	if err != nil {
		slog.Println("requestKites marshall err 2", err)
		return nil, err
	}

	remoteKites := make([]*RemoteKite, len(kitesResp))
	for i, k := range kitesResp {
		rk := &RemoteKite{
			KiteWithToken: k,
		}
		remoteKites[i] = rk
	}

	return remoteKites, nil
}

// CallSync makes a blocking request to another kite. args and result is used
// by the remote kite, therefore you should know what the kite is expecting.
func (r *Remote) CallSync(method string, args interface{}, result interface{}) error {
	remoteKite, err := r.getClient()
	if err != nil {
		return err
	}

	rpcMethod := r.Kitename + "." + method
	err = remoteKite.Client.Call(rpcMethod, args, result)
	if err != nil {
		return fmt.Errorf("can't call '%s', err: %s", r.Kitename, err.Error())
	}

	return nil
}

// Call makes a non-blocking request to another kite. args is used by the
// remote kite, therefore you should know what the kite is expecting.  fn is a
// callback that is executed when the result and error has been received.
// Currently only string as a result is supported, but it needs to be changed.
func (r *Remote) Call(method string, args interface{}, fn func(err error, res string)) (*rpc.Call, error) {
	remoteKite, err := r.getClient()
	if err != nil {
		return nil, err
	}

	var response string

	request := &protocol.KiteRequest{
		Kite:  remoteKite.Kite,
		Args:  args,
		Token: remoteKite.Token,
	}

	rpcMethod := r.Kitename + "." + method
	d := remoteKite.Client.Go(rpcMethod, request, &response, nil)

	select {
	case <-d.Done:
		fn(d.Error, response)
	case <-time.Tick(10 * time.Second):
		fn(d.Error, response)
	}

	return d, nil
}

func (r *Remote) getClient() (*RemoteKite, error) {
	kite, err := r.roundRobin()
	if err != nil {
		return nil, err
	}

	if kite.Client == nil {
		var err error

		slog.Printf("establishing HTTP client conn for %s - %s on %s\n",
			kite.Name, kite.Addr(), kite.Hostname)

		kite.Client, err = r.dialRemote(kite.Addr())
		if err != nil {
			return nil, err
		}

		// update kite in storage after we have an established connection
		kites.Add(&kite.Kite)
	}

	return kite, nil
}

func (r *Remote) roundRobin() (*RemoteKite, error) {
	if len(r.Kites) == 0 {
		return nil, fmt.Errorf("kite %s/%s does not exist", r.Username, r.Kitename)
	}

	// TODO: use container/ring :)
	index := balance.GetIndex(r.Kitename)
	N := float64(len(r.Kites))
	n := int(math.Mod(float64(index+1), N))
	balance.AddOrUpdateIndex(r.Kitename, n)
	return r.Kites[n], nil
}

// dialRemote is used to connect to a Remote Kite via the GOB codec. This is
// used by other external kite methods.
func (r *Remote) dialRemote(addr string) (*rpc.Client, error) {
	var err error
	conn, err := net.Dial("tcp4", addr)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+rpc.DefaultRPCPath+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		codec := NewKiteClientCodec(conn) // pass our custom codec
		return rpc.NewClientWithCodec(codec), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  "tcp " + addr,
		Addr: nil,
		Err:  err,
	}
}
