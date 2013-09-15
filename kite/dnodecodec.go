// Dnode protocol for net/rpc
package kite

import (
	"encoding/json"
	"fmt"
	"io"
	"koding/tools/dnode"
	"strings"
	"unicode"
	"unicode/utf8"

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

type DnodeServerCodec struct {
	dec   *json.Decoder // for reading JSON values
	enc   *json.Encoder // for writing JSON values
	rwc   io.ReadWriteCloser
	dnode *dnode.DNode
	req   dnode.Message
}

func NewDnodeServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	d := dnode.New()

	// For what is this used?
	d.OnRootMethod = func(method string, args *dnode.Partial) {
		var partials []*dnode.Partial
		err := args.Unmarshal(&partials)
		if err != nil {
			panic(err)
		}

		var options struct {
			WithArgs *dnode.Partial
		}
		err = partials[0].Unmarshal(&options)
		if err != nil {
			panic(err)
		}
		var resultCallback dnode.Callback
		err = partials[1].Unmarshal(&resultCallback)
		if err != nil {
			panic(err)
		}
	}

	return &DnodeServerCodec{
		rwc:   conn,
		dec:   json.NewDecoder(conn),
		enc:   json.NewEncoder(conn),
		dnode: d,
	}
}

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	err := c.dec.Decode(&c.req)
	if err != nil {
		return err
	}
	fmt.Printf("[received] <- %+v, %+v\n", c.req.Method, string(c.req.Arguments.Raw))

	c.dnode.ProcessDnode(c.req)
	method := upperFirst(strings.Split(c.req.Method.(string), ".")[1])
	r.ServiceMethod = method

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
	c.dnode.Close()
	return c.rwc.Close()
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
