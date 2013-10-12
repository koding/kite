// Implements a modified JSON-RPC ClientCodec and ServerCodec that uses Kontrol
// authentication and Kite protocol for the rpc package.
package kite

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"net/rpc"
	"sync"
)

/******************************************

Client

******************************************/

type clientRequest struct {
	Method string         `json:"method"`
	Params [1]interface{} `json:"params"`
	Id     uint64         `json:"id"`
}

type clientResponse struct {
	Id     uint64           `json:"id"`
	Result *json.RawMessage `json:"result"`
	Error  interface{}      `json:"error"`
}

func (r *clientResponse) reset() {
	r.Id = 0
	r.Result = nil
	r.Error = nil
}

type JsonClientCodec struct {
	dec *json.Decoder // for reading JSON values
	enc *json.Encoder // for writing JSON values
	c   io.Closer

	// temporary work space
	req  clientRequest
	resp clientResponse

	// JSON-RPC responses include the request id but not the request method.
	// Package rpc expects both.
	// We save the request method in pending when sending a request
	// and then look it up by request ID when filling out the rpc Response.
	mutex   sync.Mutex        // protects pending
	pending map[uint64]string // map request id to method name

	// add our own information
	Kite *Kite
}

func NewJsonClientCodec(kite *Kite, conn io.ReadWriteCloser) rpc.ClientCodec {
	return &JsonClientCodec{
		dec:     json.NewDecoder(conn),
		enc:     json.NewEncoder(conn),
		c:       conn,
		pending: make(map[uint64]string),
		Kite:    kite,
	}
}

func (c *JsonClientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	slog.Println("JsonClient WriteRequest")

	c.mutex.Lock()
	c.pending[r.Seq] = r.ServiceMethod
	c.mutex.Unlock()
	c.req.Method = r.ServiceMethod
	c.req.Params[0] = param
	c.req.Id = r.Seq
	return c.enc.Encode(&c.req)
}

func (c *JsonClientCodec) ReadResponseHeader(r *rpc.Response) error {
	slog.Println("JsonClient ReadResponseHeader")

	c.resp.reset()
	if err := c.dec.Decode(&c.resp); err != nil {
		return err
	}

	c.mutex.Lock()
	r.ServiceMethod = c.pending[c.resp.Id]
	delete(c.pending, c.resp.Id)
	c.mutex.Unlock()

	r.Error = ""
	r.Seq = c.resp.Id
	if c.resp.Error != nil || c.resp.Result == nil {
		x, ok := c.resp.Error.(string)
		if !ok {
			return fmt.Errorf("invalid error %v", c.resp.Error)
		}
		if x == "" {
			x = "unspecified error"
		}
		r.Error = x
	}
	return nil
}

func (c *JsonClientCodec) ReadResponseBody(x interface{}) error {
	slog.Println("Client ReadResponseBody")
	if x == nil {
		return nil
	}
	return json.Unmarshal(*c.resp.Result, x)
}

func (c *JsonClientCodec) Close() error {
	return c.c.Close()
}

/******************************************

SERVER

******************************************/

type serverRequest struct {
	Method    string           `json:"method"`
	Params    *json.RawMessage `json:"params"`
	Id        *json.RawMessage `json:"id"`
	Callbacks []string         `json:"callbacks"`
	Username  string           `json:"username"`
	Kitename  string           `json:"kitename"`
	Token     string           `json:"token"`
	Origin    string           `jsong:"-"`
}

func (r *serverRequest) reset() {
	r.Method = ""
	r.Params = nil
	r.Id = nil
	r.Callbacks = nil
	r.Username = ""
	r.Kitename = ""
	r.Token = ""
}

type serverResponse struct {
	Id     *json.RawMessage `json:"id"`
	Result interface{}      `json:"result"`
	Error  interface{}      `json:"error"`
}

type JsonServerCodec struct {
	dec *json.Decoder // for reading JSON values
	enc *json.Encoder // for writing JSON values
	c   io.Closer

	// temporary work space
	req  serverRequest
	resp serverResponse

	// JSON-RPC clients can use arbitrary json values as request IDs.
	// Package rpc expects uint64 request IDs.
	// We assign uint64 sequence numbers to incoming requests
	// but save the original request ID in the pending map.
	// When rpc responds, we use the sequence number in
	// the response to find the original request ID.
	mutex   sync.Mutex // protects seq, pending
	seq     uint64
	pending map[uint64]*json.RawMessage
	kite    *Kite
}

func NewJsonServerCodec(kite *Kite, conn io.ReadWriteCloser) rpc.ServerCodec {
	return &JsonServerCodec{
		dec:     json.NewDecoder(conn),
		enc:     json.NewEncoder(conn),
		c:       conn,
		pending: make(map[uint64]*json.RawMessage),
		kite:    kite,
	}
}

func (c *JsonServerCodec) ReadRequestHeader(r *rpc.Request) error {
	slog.Println("JsonServer ReadRequestHeader")

	c.req.reset()
	if err := c.dec.Decode(&c.req); err != nil {
		return err
	}

	r.ServiceMethod = c.req.Method

	// get underlying websocket connection and update our clients with the
	// request data. that means remove it from the buffer list(bufClients) and
	// add it to the registered user list (clients).
	// be aware that this method is called only when a RPC call is made, that
	// means this is not called when a connection is established
	if c.req.Username != "" {
		ws := c.c.(*websocket.Conn)
		addr := ws.Request().RemoteAddr
		client := c.kite.Clients.Get(&client{Addr: addr})
		if client != nil {
			client.Username = c.req.Username
			c.kite.Clients.Add(client)
		}
	}

	c.req.Origin = protocol.ORIGIN_JSON
	// JSON request id can be any JSON value;
	// RPC package expects uint64.  Translate to
	// internal uint64 and save JSON on the side.
	c.mutex.Lock()
	c.seq++
	c.pending[c.seq] = c.req.Id
	c.req.Id = nil
	r.Seq = c.seq
	c.mutex.Unlock()

	return nil
}

var errMissingParams = errors.New("jsonrpc: request body missing params")

func (c *JsonServerCodec) ReadRequestBody(x interface{}) error {
	slog.Println("JsonServer ReadRequestBody")
	if x == nil {
		return nil
	}
	if c.req.Params == nil {
		return errMissingParams
	}

	// JSON params is array value.
	// RPC params is struct.
	// Unmarshal into array containing struct for now.
	// Should think about making RPC more general.

	// We use our custom RequestBody protocol, convert and pass the Args
	a := x.(*protocol.KiteRequest)
	a.Username = c.req.Username
	a.Kitename = c.req.Kitename
	a.Token = c.req.Token
	a.Method = c.req.Method
	a.Origin = c.req.Origin

	var params [1]interface{}
	params[0] = &a.Args

	slog.Printf("got a call request from %s with token %s: -> ", a.Kitename, a.Token)
	if permissions.Has(a.Token) {
		slog.Printf("... already allowed to run\n")
		return json.Unmarshal(*c.req.Params, &params)
	}

	m := protocol.Request{
		Base: protocol.Base{
			Username: a.Username,
			Token:    a.Token,
		},
		RemoteKite: a.Kitename,
		Action:     "getPermission",
	}

	msg, _ := json.Marshal(&m)

	slog.Printf("\nasking kontrol for permission, for '%s' with token '%s': -> ", a.Kitename, a.Token)
	result := c.kite.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	json.Unmarshal(result, &resp)

	switch resp.Result {
	case protocol.AllowKite:
		slog.Printf("... allowed to run\n")
		permissions.Add(a.Token)
		return json.Unmarshal(*c.req.Params, &params)
	case protocol.PermitKite:
		slog.Printf("... not allowed. permission denied via Kontrol\n")
		return errors.New("no permission to run")
	}

	return errors.New("got a nonstandart response")
}

var null = json.RawMessage([]byte("null"))

func (c *JsonServerCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	slog.Println("JsonServer WriteRequest")
	var resp serverResponse
	c.mutex.Lock()
	b, ok := c.pending[r.Seq]
	if !ok {
		c.mutex.Unlock()
		return errors.New("invalid sequence number in response")
	}
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	if b == nil {
		// Invalid request so no id.  Use JSON null.
		b = &null
	}
	resp.Id = b
	resp.Result = x
	if r.Error == "" {
		resp.Error = nil
	} else {
		resp.Error = r.Error
	}
	return c.enc.Encode(resp)
}

func (c *JsonServerCodec) Close() error {
	return c.c.Close()
}
