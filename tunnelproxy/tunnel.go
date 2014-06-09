package tunnelproxy

import (
	"io"
	"sync"

	"code.google.com/p/go.net/websocket"
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
	<-JoinStreams(t.localConn, remoteConn)
	t.Close()
}

func JoinStreams(local, remote io.ReadWriteCloser) chan error {
	errc := make(chan error, 2)

	copy := func(dst io.WriteCloser, src io.ReadCloser) {
		_, err := io.Copy(dst, src)
		src.Close()
		dst.Close()
		errc <- err
	}

	go copy(local, remote)
	go copy(remote, local)

	return errc
}
