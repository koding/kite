package kite

import (
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/protocol"
	"net"
	"sync"
)

// Kontrol embeds RemoteKite which has additional special helper methods.
type Kontrol struct {
	RemoteKite

	// used for synchronizing methods that needs to be called after
	// successful connection.
	ready chan bool
}

// NewKontrol returns a pointer to new Kontrol instance.
func (k *Kite) NewKontrol(addr string) *Kontrol {
	// Only the address is required to connect Kontrol
	host, port, _ := net.SplitHostPort(addr)
	kite := protocol.Kite{
		PublicIP: host,
		Port:     port,
		Name:     "kontrol", // for logging purposes
	}

	auth := callAuthentication{
		Type: "kodingKey",
		Key:  k.KodingKey,
	}

	remoteKite := k.NewRemoteKite(kite, auth)
	remoteKite.client.Reconnect = true

	var once sync.Once
	ready := make(chan bool)

	remoteKite.OnConnect(func() {
		k.Log.Info("Connected to Kontrol ")

		// signal all other methods that are listening on this channel, that we
		// are ready.
		once.Do(func() { close(ready) })
	})

	remoteKite.OnDisconnect(func() { k.Log.Warning("Disconnected from Kontrol. I will retry in background...") })

	return &Kontrol{
		RemoteKite: *remoteKite,
		ready:      ready,
	}
}

// Register registers current Kite to Kontrol. After registration other Kites
// can find it via GetKites() method.
func (k *Kontrol) Register() error {
	response, err := k.RemoteKite.Call("register", nil)
	if err != nil {
		return err
	}

	var rr protocol.RegisterResult
	err = response.Unmarshal(&rr)
	if err != nil {
		return err
	}

	switch rr.Result {
	case protocol.AllowKite:
		kite := &k.localKite.Kite

		// we know now which user that is after authentication
		kite.Username = rr.Username

		// Set the correct PublicIP if left empty in options.
		if kite.PublicIP == "" {
			kite.PublicIP = rr.PublicIP
		}

		k.Log.Info("Registered to kontrol with addr: %s version: %s uuid: %s",
			kite.Addr(), kite.Version, kite.ID)
	case protocol.RejectKite:
		return errors.New("Kite rejected")
	default:
		return fmt.Errorf("Invalid result: %s", rr.Result)
	}

	return nil
}

// GetKites returns the list of Kites matching the query.
// The returned list contains ready to connect RemoteKite instances.
// The caller must connect with RemoteKite.Dial() before using each Kite.
func (k *Kontrol) GetKites(query protocol.KontrolQuery, onEvent func(*protocol.KiteEvent)) ([]*RemoteKite, error) {
	// this is needed because we are calling GetKites explicitly, therefore
	// this should be only callable *after* we are connected to kontrol.
	<-k.ready

	queueEvents := func(p *dnode.Partial) {
		var args []*dnode.Partial
		err := p.Unmarshal(&args)
		if err != nil {
			k.Log.Error(err.Error())
			return
		}

		if len(args) != 1 {
			k.Log.Error("Invalid Kite event")
			return
		}

		var event protocol.KiteEvent
		err = args[0].Unmarshal(&event)
		if err != nil {
			k.Log.Error(err.Error())
			return
		}

		onEvent(&event)
	}

	args := []interface{}{query}
	if onEvent != nil {
		args = append(args, dnode.Callback(queueEvents))
	}

	response, err := k.RemoteKite.Call("getKites", args)
	if err != nil {
		return nil, err
	}

	var kites []protocol.KiteWithToken
	err = response.Unmarshal(&kites)
	if err != nil {
		return nil, err
	}

	remoteKites := make([]*RemoteKite, len(kites))
	for i, kite := range kites {
		auth := callAuthentication{
			Type: "token",
			Key:  kite.Token,
		}

		remoteKites[i] = k.localKite.NewRemoteKite(kite.Kite, auth)
	}

	return remoteKites, nil
}

func (k *Kontrol) GetToken(kite *protocol.Kite) (string, error) {
	<-k.ready

	result, err := k.RemoteKite.Call("getToken", kite)
	if err != nil {
		return "", err
	}

	return result.MustString(), nil
}
