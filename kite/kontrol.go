package kite

import (
	"errors"
	"fmt"
	"koding/newkite/protocol"
)

const (
	KontrolHost = "127.0.0.1"
	KontrolPort = "4000"
)

// Kontrol is also a Kite which has special helper methods.
type Kontrol struct{ RemoteKite }

// NewKontrol returns a pointer to new Kontrol instance.
func (k *Kite) NewKontrol() *Kontrol {
	// Only the address is required to connect Kontrol
	kite := protocol.Kite{
		PublicIP: KontrolHost,
		Port:     KontrolPort,
	}

	auth := callAuthentication{
		Type: "kodingKey",
		Key:  k.KodingKey,
	}

	remoteKite := k.NewRemoteKite(kite, auth)
	remoteKite.Client.Reconnect = true

	// Print log messages on connect/disconnect.
	remoteKite.Client.OnConnect(func() { log.Info("Connected to Kontrol.") })
	remoteKite.Client.OnDisconnect(func() { log.Warning("Disconnected from Kontrol. I will retry in background...") })

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
func (k *Kontrol) GetKites(query protocol.KontrolQuery) ([]*RemoteKite, error) {
	response, err := k.RemoteKite.Call("getKites", query)
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
