// Dnode protocol for net/rpc
package kite

import (
	"encoding/json"
	"fmt"
	"io"
	"koding/newkite/protocol"
	"koding/tools/dnode"
	"log"
	"net/rpc"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	dec    *json.Decoder // for reading JSON values
	enc    *json.Encoder // for writing JSON values
	rwc    io.ReadWriteCloser
	dnode  *dnode.DNode
	req    dnode.Message
	result dnode.Callback
}

func NewDnodeServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &DnodeServerCodec{
		rwc:   conn,
		dec:   json.NewDecoder(conn),
		enc:   json.NewEncoder(conn),
		dnode: dnode.New(),
	}
}

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	err := c.dec.Decode(&c.req)
	if err != nil {
		return err
	}

	fmt.Printf("[received] <- %+v, %+v\n", c.req.Method, string(c.req.Arguments.Raw))

	// m -> c.req
	// m.Arguments -> c.req.Arguments
	for id, path := range c.req.Callbacks {
		methodId, err := strconv.Atoi(id)
		if err != nil {
			panic(err)
		}

		// TODO: dnode.Callback should return error
		callback := dnode.Callback(func(args ...interface{}) {
			callbacks := make(map[string]([]string))
			c.dnode.CollectCallbacks(args, make([]string, 0), callbacks)

			rawArgs, err := json.Marshal(args)
			if err != nil {
				log.Println(err)
			}

			message := dnode.Message{
				Method:    methodId,
				Arguments: &dnode.Partial{Raw: rawArgs},
				Links:     []string{}, // Links is not used, send only for compatiblity
				Callbacks: callbacks,
			}

			fmt.Printf("[sending] -> %+v, %+v\n", c.req.Method, message)
			c.enc.Encode(message)
		})
		c.req.Arguments.Callbacks = append(c.req.Arguments.Callbacks, dnode.CallbackSpec{path, callback})
	}

	method := upperFirst(strings.Split(c.req.Method.(string), ".")[1])

	r.ServiceMethod = "os-local." + method // TODO: don't make it hardcoded
	r.Seq = 0                              // TODO: should have dnode callback id

	return nil
}

func (c *DnodeServerCodec) ReadRequestBody(body interface{}) error {
	if body == nil {
		return nil
	}

	a := body.(*protocol.KiteRequest)
	// args  is of type *dnode.Partial
	var partials []*dnode.Partial

	err := c.req.Arguments.Unmarshal(&partials)
	if err != nil {
		return err
	}

	var options struct {
		WithArgs *dnode.Partial
	}

	err = partials[0].Unmarshal(&options)
	if err != nil {
		return err
	}
	a.ArgsDnode = options.WithArgs

	var resultCallback dnode.Callback
	err = partials[1].Unmarshal(&resultCallback)
	if err != nil {
		return err
	}

	c.result = resultCallback
	return nil
}

func (c *DnodeServerCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	if r.Error != "" {
		return fmt.Errorf(r.Error)
	}

	c.result(nil, body)
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
