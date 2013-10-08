package kite

import (
	"bytes"
	zmq "github.com/pebbe/zmq3"
	"koding/newkite/protocol"
	"sync"
)

// ZeroMQ is a struct that complies with the Messenger interface.
type ZeroMQ struct {
	Subscriber *zmq.Socket
	Dealer     *zmq.Socket
	sync.Mutex // protects zmq send for DEALER socket
}

func NewZeroMQ(kiteID string) *ZeroMQ {
	routerKontrol := "tcp://127.0.0.1:5556"
	subKontrol := "tcp://127.0.0.1:5557"
	sub, _ := zmq.NewSocket(zmq.SUB)
	sub.Connect(subKontrol)

	dealer, _ := zmq.NewSocket(zmq.DEALER)
	dealer.SetIdentity(kiteID) // use our ID also for zmq envelope
	dealer.Connect(routerKontrol)

	return &ZeroMQ{
		Subscriber: sub,
		Dealer:     dealer,
	}
}

func (z *ZeroMQ) Subscribe(filter string) error {
	return z.Subscriber.SetSubscribe(filter)
}

func (z *ZeroMQ) Unsubscribe(filter string) error {
	return z.Subscriber.SetUnsubscribe(filter)
}

func (z *ZeroMQ) Send(msg []byte) []byte {
	z.Lock()
	defer z.Unlock()

	z.Dealer.SendBytes([]byte(""), zmq.SNDMORE)
	z.Dealer.SendBytes(msg, 0)

	z.Dealer.RecvBytes(0) // envelope delimiter
	reply, _ := z.Dealer.RecvBytes(0)
	return reply
}

func (z *ZeroMQ) Consume(handle func([]byte)) {
	for {
		msg, _ := z.Subscriber.RecvBytes(0)
		frames := bytes.SplitN(msg, []byte(protocol.FRAME_SEPARATOR), 2)
		if len(frames) != 2 { // msg is malformed
			continue
		}

		// frames[0] contains the filter string
		// frames[1] contains the msg content
		handle(frames[1])
	}
}
