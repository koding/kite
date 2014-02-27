package proxy

import (
	"sync"

	"code.google.com/p/go.net/websocket"
	"github.com/koding/kite/util"
)

type Tunnel struct {
	id          uint64          // key in kites's tunnels map
	localConn   *websocket.Conn // conn to local kite
	startChan   chan bool       // to signal started state
	closeChan   chan bool       // to signal closed state
	closed      bool            // to prevent closing closeChan again
	closedMutex sync.Mutex      // for protection of closed field
}

func (t *Tunnel) Close() {
	t.closedMutex.Lock()
	defer t.closedMutex.Unlock()

	if t.closed {
		return
	}

	t.localConn.Close()
	close(t.closeChan)
	t.closed = true
}

func (t *Tunnel) CloseNotify() chan bool {
	return t.closeChan
}

func (t *Tunnel) StartNotify() chan bool {
	return t.startChan
}

func (t *Tunnel) Run(remoteConn *websocket.Conn) {
	close(t.startChan)
	<-util.JoinStreams(t.localConn, remoteConn)
	t.Close()
}
