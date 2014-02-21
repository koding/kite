package kite

import (
	"fmt"
	"github.com/koding/kite/systeminfo"
	"github.com/koding/kite/util"
	"net/url"
	"os/exec"
	"time"

	"code.google.com/p/go.net/websocket"
)

// systemInfo returns info about the system (CPU, memory, disk...).
func systemInfo(r *Request) (interface{}, error) {
	return systeminfo.New()
}

// handleHeartbeat pings the callback with the given interval seconds.
func (k *Kite) handleHeartbeat(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	seconds := args[0].MustFloat64()
	ping := args[1].MustFunction()

	go func() {
		for {
			time.Sleep(time.Duration(seconds) * time.Second)
			if ping() != nil {
				return
			}
		}
	}()

	return nil, nil
}

// handleLog prints a log message to stderr.
func (k *Kite) handleLog(r *Request) (interface{}, error) {
	msg := r.Args.One().MustString()
	k.Log.Info(fmt.Sprintf("%s: %s", r.RemoteKite.Name, msg))
	return nil, nil
}

// handlePrint prints a message to stdout.
func handlePrint(r *Request) (interface{}, error) {
	return fmt.Print(r.Args.One().MustString())
}

// handlePrompt asks user a single line input.
func handlePrompt(r *Request) (interface{}, error) {
	fmt.Print(r.Args.One().MustString())
	var s string
	_, err := fmt.Scanln(&s)
	return s, err
}

// handleNotifyDarwin displays a desktop notification on OS X.
func handleNotifyDarwin(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(3)
	cmd := exec.Command("osascript", "-e", fmt.Sprintf("display notification \"%s\" with title \"%s\" subtitle \"%s\"",
		args[1].MustString(), args[2].MustString(), args[0].MustString()))
	return nil, cmd.Start()
}

// handleTunnel opens two websockets, one to proxy kite and one to itself,
// then it copies the message between them.
func handleTunnel(r *Request) (interface{}, error) {
	var args struct {
		URL string
	}
	r.Args.One().MustUnmarshal(&args)

	parsed, err := url.Parse(args.URL)
	if err != nil {
		return nil, err
	}

	conf := &websocket.Config{
		Location:  parsed,
		Version:   websocket.ProtocolVersionHybi13,
		Origin:    &url.URL{Scheme: "http", Host: "localhost"},
		TlsConfig: r.LocalKite.tlsConfig(),
	}

	remoteConn, err := websocket.DialConfig(conf)
	if err != nil {
		return nil, err
	}

	conf.Location = r.LocalKite.ServingURL

	localConn, err := websocket.DialConfig(conf)
	if err != nil {
		return nil, err
	}

	util.JoinStreams(localConn, remoteConn)
	return nil, nil
}
