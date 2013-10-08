package kite

import (
	"bytes"
	zmq "github.com/pebbe/zmq3"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"sync"
)

// ZeroMQ is a struct that complies with the Messenger interface.
type ZeroMQ struct {
	UUID     string
	Kitename string
	All      string

	Subscriber *zmq.Socket
	Dealer     *zmq.Socket
	sync.Mutex // protects zmq send for DEALER socket
}

func NewZeroMQ(kiteID, kitename, all string) *ZeroMQ {
	routerKontrol := "tcp://127.0.0.1:5556"
	subKontrol := "tcp://127.0.0.1:5557"
	sub, _ := zmq.NewSocket(zmq.SUB)
	sub.Connect(subKontrol)

	// set three filters
	sub.SetSubscribe(kiteID)   // individual, just for me
	sub.SetSubscribe(kitename) // same type, kites with the same name
	sub.SetSubscribe(all)      // for all kites

	dealer, _ := zmq.NewSocket(zmq.DEALER)
	dealer.SetIdentity(kiteID) // use our ID also for zmq envelope
	dealer.Connect(routerKontrol)

	return &ZeroMQ{
		Subscriber: sub,
		Dealer:     dealer,
		UUID:       kiteID,
		Kitename:   kitename,
		All:        all,
	}
}

func (z *ZeroMQ) Send(msg []byte) []byte {
	z.Lock()

	z.Dealer.SendBytes([]byte(""), zmq.SNDMORE)
	z.Dealer.SendBytes(msg, 0)

	z.Dealer.RecvBytes(0) // envelope delimiter
	reply, _ := z.Dealer.RecvBytes(0)
	z.Unlock()
	return reply
}

func (z *ZeroMQ) Consume(handle func([]byte)) {
	for {
		msg, _ := z.Subscriber.RecvBytes(0)
		frames := bytes.SplitN(msg, []byte(protocol.FRAME_SEPARATOR), 2)
		if len(frames) != 2 { // msg is malformed
			continue
		}

		filter := frames[0] // either "all" or k.Uuid (just for this kite)
		switch string(filter) {
		case z.All, z.UUID, z.Kitename:
			msg = frames[1] // msg is in JSON format
			handle(msg)
		default:
			slog.Println("not intended for me, dropping", string(filter))
		}

	}
}
