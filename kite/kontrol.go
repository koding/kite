package kite

import (
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/protocol"
	"net"
)

// Kontrol is also a Kite which has special helper methods.
type Kontrol struct{ RemoteKite }

// NewKontrol returns a pointer to new Kontrol instance.
func (k *Kite) NewKontrol(addr string) *Kontrol {
	// Only the address is required to connect Kontrol
	host, port, _ := net.SplitHostPort(addr)
	kite := protocol.Kite{
		PublicIP: host,
		Port:     port,
	}

	auth := callAuthentication{
		Type: "kodingKey",
		Key:  k.KodingKey,
	}

	remoteKite := k.NewRemoteKite(kite, auth)
	remoteKite.client.Reconnect = true

	// Print log messages on connect/disconnect.
	remoteKite.OnConnect(func() { log.Info("Connected to Kontrol.") })
	remoteKite.OnDisconnect(func() { log.Warning("Disconnected from Kontrol. I will retry in background...") })

	return &Kontrol{*remoteKite}
}

// Register registers current Kite to Kontrol.
// After registration other Kites can find it via GetKites() method.
func (k *Kontrol) Register() error {
	log.Debug("Registering to Kontrol")
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
		kite := &k.Kite
		log.Info("registered to kontrol with addr: %s version: %s uuid: %s",
			kite.Addr(), kite.Version, kite.ID)

		// we know now which user that is
		kite.Username = rr.Username

		// Set the correct PublicIP if left empty in options.
		if kite.PublicIP == "" {
			kite.PublicIP = rr.PublicIP
		}
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
func (k *Kontrol) GetKites(query protocol.KontrolQuery, events chan *protocol.KiteEvent) ([]*RemoteKite, error) {
	queueEvents := func(p *dnode.Partial) {
		var args []*dnode.Partial
		err := p.Unmarshal(&args)
		if err != nil {
			log.Error(err.Error())
			return
		}

		if len(args) != 1 {
			log.Error("Invalid Kite event")
			return
		}

		var event protocol.KiteEvent
		err = args[0].Unmarshal(&event)
		if err != nil {
			log.Error(err.Error())
			return
		}

		events <- &event
	}

	args := []interface{}{query}
	if events != nil {
		k.OnDisconnect(func() { close(events) })
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
