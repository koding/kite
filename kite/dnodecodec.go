// Dnode protocol for net/rpc
package kite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/rpc"
)

func NewDnodeClient(conn io.ReadWriteCloser) rpc.ClientCodec {
	return &DnodeClientCodec{
		rwc: conn,
		dec: json.NewDecoder(conn),
		enc: json.NewEncoder(conn),
	}
}

type DnodeClientCodec struct {
	dec *json.Decoder
	enc *json.Encoder
	rwc io.ReadWriteCloser
}

func (c *DnodeClientCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	fmt.Println("Dnode WriteRequest")

	return nil
}

func (c *DnodeClientCodec) ReadResponseHeader(r *rpc.Response) error {
	fmt.Println("Dnode ReadResponseHeader")
	return nil
}

func (c *DnodeClientCodec) ReadResponseBody(x interface{}) error {
	fmt.Println("Dnode ReadResponseBody")
	return nil
}

func (c *DnodeClientCodec) Close() error {
	fmt.Println("Dnode ClientClose")
	return nil
}

type DnodeServerCodec struct{}

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	fmt.Println("Dnode ReadRequestHeader")
	return nil
}

func (c *DnodeServerCodec) ReadRequestBody(body interface{}) error {
	fmt.Println("Dnode ReadRequestBody")
	return nil
}

func (c *DnodeServerCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	fmt.Println("Dnode WriteResponse")
	return nil
}

func (c *DnodeServerCodec) Close() error {
	fmt.Println("Dnode ServerClose")
	return nil
}
