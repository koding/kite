// Implements a modified GOB ClientCodec and ServerCodec that uses Kontrol
// authentication and Kite protocol for the rpc package.
package kite

import (
	"bufio"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io"
	"koding/newkite/protocol"
	"net/rpc"
)

/******************************************

Client

******************************************/

type KiteClientCodec struct {
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
	Kite   *Kite
}

func NewKiteClientCodec(kite *Kite, conn io.ReadWriteCloser) rpc.ClientCodec {
	buf := bufio.NewWriter(conn)
	c := &KiteClientCodec{
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
		Kite:   kite,
	}

	return c
}

func (c *KiteClientCodec) WriteRequest(r *rpc.Request, body interface{}) (err error) {
	debug("Client WriteRequest")
	if err = c.enc.Encode(r); err != nil {
		return
	}

	if err = c.enc.Encode(body); err != nil {
		return
	}

	return c.encBuf.Flush()
}

func (c *KiteClientCodec) ReadResponseHeader(r *rpc.Response) error {
	debug("Client ReadResponseHeader")
	return c.dec.Decode(r)
}

func (c *KiteClientCodec) ReadResponseBody(body interface{}) error {
	debug("Client ReadResponseBody")
	return c.dec.Decode(body)
}

func (c *KiteClientCodec) Close() error {
	return c.rwc.Close()
}

/******************************************

SERVER

******************************************/

type KiteServerCodec struct {
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
	Kite   *Kite
}

func NewKiteServerCodec(kite *Kite, conn io.ReadWriteCloser) rpc.ServerCodec {
	buf := bufio.NewWriter(conn)
	c := &KiteServerCodec{
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
		Kite:   kite,
	}

	return c
}

func (c *KiteServerCodec) ReadRequestHeader(r *rpc.Request) error {
	debug("Server ReadRequestHeader")
	return c.dec.Decode(r)
}

func (c *KiteServerCodec) ReadRequestBody(body interface{}) error {
	debug("Server ReadRequestBody")
	if body == nil {
		return c.dec.Decode(body)
	}

	a := body.(*protocol.KiteRequest)
	err := c.dec.Decode(a)
	if err != nil {
		return err
	}

	debug("got a call request from %s with token %s: -> ", a.Kitename, a.Token)
	if permissions.Has(a.Token) {
		debug("... already allowed to run\n")
		return nil
	}

	m := protocol.Request{
		Base: protocol.Base{
			Username: c.Kite.Username,
			Kitename: c.Kite.Kitename,
			Token:    a.Token,
			Uuid:     a.Uuid,
			Hostname: c.Kite.Hostname,
			Addr:     c.Kite.Addr,
		},
		RemoteKite: a.Kitename,
		Action:     "getPermission",
	}

	msg, _ := json.Marshal(&m)

	debug("\nasking kontrol for permission, for '%s' with token '%s': -> ", a.Kitename, a.Token)
	result := c.Kite.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	json.Unmarshal(result, &resp)

	switch resp.Result {
	case protocol.AllowKite:
		debug("... allowed to run\n")
		permissions.Add(a.Token)
		return nil
	case protocol.PermitKite:
		debug("... not allowed. permission denied via Kontrol\n")
		return errors.New("not allowed to start")
	default:
		return errors.New("got a nonstandart response")
	}

	return nil
}

func (c *KiteServerCodec) WriteResponse(r *rpc.Response, body interface{}) (err error) {
	debug("Server WriteRequest")
	if err = c.enc.Encode(r); err != nil {
		return
	}
	if err = c.enc.Encode(body); err != nil {
		return
	}
	return c.encBuf.Flush()
}

func (c *KiteServerCodec) Close() error {
	return c.rwc.Close()
}
