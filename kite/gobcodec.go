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
	"koding/tools/slog"
	"net"
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

func NewKiteClientCodec(conn io.ReadWriteCloser) rpc.ClientCodec {
	buf := bufio.NewWriter(conn)
	c := &KiteClientCodec{
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
	}

	return c
}

func (c *KiteClientCodec) WriteRequest(r *rpc.Request, body interface{}) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return
	}

	if err = c.enc.Encode(body); err != nil {
		return
	}

	return c.encBuf.Flush()
}

func (c *KiteClientCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.dec.Decode(r)
}

func (c *KiteClientCodec) ReadResponseBody(body interface{}) error {
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
	return c.dec.Decode(r)
}

func (c *KiteServerCodec) ReadRequestBody(body interface{}) error {
	if body == nil {
		return c.dec.Decode(body)
	}

	a := body.(*protocol.KiteRequest)
	err := c.dec.Decode(a)
	if err != nil {
		return err
	}

	// Return when kontrol is not enabled
	if !c.Kite.KontrolEnabled {
		return nil
	}

	if permissions.Has(a.Token) {
		slog.Printf("[%s] allowed token (cached) '%s'\n",
			c.rwc.(net.Conn).RemoteAddr().String(), a.Token)
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
	slog.Printf("asking kontrol if token '%s' from %s is valid\n", a.Token, a.Username)

	msg, _ := json.Marshal(&m)
	result := c.Kite.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	json.Unmarshal(result, &resp)

	switch resp.Result {
	case protocol.AllowKite:
		slog.Printf("[%s] allowed token '%s'\n", c.rwc.(net.Conn).RemoteAddr(), a.Token)
		permissions.Add(a.Token)
		return nil
	case protocol.PermitKite:
		slog.Printf("denied token '%s'\n", a.Token)
		return errors.New("no permission to run")
	}

	return errors.New("got a nonstandart response")
}

func (c *KiteServerCodec) WriteResponse(r *rpc.Response, body interface{}) (err error) {
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
