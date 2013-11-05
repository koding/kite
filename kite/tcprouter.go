package kite

import (
	"koding/newkite/protocol"
	"net"
	"net/rpc"
)

type TCPKite struct {
	server *rpc.Server
	kite   *Kite
	Addr   string
}

func NewTCPKite(k *Kite) *TCPKite {
	return &TCPKite{
		server: rpc.NewServer(),
		kite:   k,
	}
}

func (t *TCPKite) DialClient(kite *protocol.Kite) (*rpc.Client, error) {
	addr := kite.Addr()
	log.Info("establishing TCP client conn for %s - %s on %s\n", kite.Name, addr, kite.Hostname)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Info(addr, err)
		return nil, err
	}

	c := NewKiteClientCodec(conn)
	return rpc.NewClientWithCodec(c), nil
}

func (t *TCPKite) Serve(addr string) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Info("PANIC!!!!! RPC SERVER COULD NOT INITIALIZED:", err)
		return
	}

	t.Addr = listener.Addr().String()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go t.server.ServeCodec(NewKiteServerCodec(t.kite, conn))
	}
}

func (k *TCPKite) AddFunction(name string, method interface{}) {
	k.server.RegisterName(name, method)
}
