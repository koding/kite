package tunnelproxy

import (
	"io"
	"sync"

	"github.com/igm/sockjs-go/sockjs"
)

type Tunnel struct {
	id          uint64         // key in kites's tunnels map
	localConn   sockjs.Session // conn to local kite
	startChan   chan bool      // to signal started state
	closeChan   chan bool      // to signal closed state
	closed      bool           // to prevent closing closeChan again
	closedMutex sync.Mutex     // for protection of closed field
}

func (t *Tunnel) Close() {
	t.closedMutex.Lock()
	defer t.closedMutex.Unlock()

	if t.closed {
		return
	}

	t.localConn.Close(3000, "Go away!")
	close(t.closeChan)
	t.closed = true
}

func (t *Tunnel) CloseNotify() chan bool {
	return t.closeChan
}

func (t *Tunnel) StartNotify() chan bool {
	return t.startChan
}

func (t *Tunnel) Run(remoteConn sockjs.Session) {
	close(t.startChan)
	<-JoinStreams(SessionReadWriteCloser{t.localConn}, SessionReadWriteCloser{remoteConn})
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

type SessionReadWriteCloser struct {
	session sockjs.Session
}

func (s SessionReadWriteCloser) Read(b []byte) (int, error) {
	str, err := s.session.Recv()
	if err != nil {
		return 0, err
	}
	copy(b, []byte(str))
	return len(str), nil
}

func (s SessionReadWriteCloser) Write(b []byte) (int, error) {
	return len(b), s.session.Send(string(b))
}

func (s SessionReadWriteCloser) Close() error {
	return s.session.Close(3000, "Go away!")
}
