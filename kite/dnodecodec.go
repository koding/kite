// Dnode protocol for net/rpc
package kite

import (
	"encoding/json"
	"fmt"
	"io"
	"koding/tools/dnode"
	"log"
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
	return c.rwc.Close()
}

//

type DnodeMessage struct {
	Method    interface{}           `json:"method"`
	Arguments *dnode.Partial        `json:"arguments"`
	Callbacks map[string]([]string) `json:"callbacks"`
}

func (d *DnodeMessage) reset() {
	d.Method = nil
	d.Arguments = nil
	d.Callbacks = nil
}

type DnodeServerCodec struct {
	dec *json.Decoder // for reading JSON values
	enc *json.Encoder // for writing JSON values
	rwc io.ReadWriteCloser

	// temporary work space
	req  DnodeMessage
	resp DnodeMessage
}

func NewDnodeServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &DnodeServerCodec{
		rwc: conn,
		dec: json.NewDecoder(conn),
		enc: json.NewEncoder(conn),
	}
}

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	err := c.dec.Decode(&c.req)
	if err != nil {
		log.Println(err)
	}
	fmt.Printf("Dnode Message arguments %+v\n", c.req.Arguments)
	fmt.Printf("Dnode Message arguments %+v\n", string(c.req.Arguments.Raw))

	return nil
}

func (c *DnodeServerCodec) ReadRequestBody(body interface{}) error {
	return nil
}

func (c *DnodeServerCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	// fmt.Printf("Dnode WriteResponse r: %+v body: %+v\n", r, body)
	return nil
}

func (c *DnodeServerCodec) Close() error {
	return c.rwc.Close()
}
