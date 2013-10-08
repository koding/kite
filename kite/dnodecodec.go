// Dnode protocol for net/rpc
package kite

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"koding/newkite/protocol"
	"koding/tools/dnode"
	"net/rpc"
	"reflect"
	"strconv"
)

// TODO: Needs to be implemented.
func NewDnodeClient(kite *Kite, conn io.ReadWriteCloser) rpc.ClientCodec {
	return &DnodeClientCodec{
		rwc: conn,
		dec: json.NewDecoder(conn),
		enc: json.NewEncoder(conn),
	}
}

type DnodeClientCodec struct {
	dec   *json.Decoder
	enc   *json.Encoder
	rwc   io.ReadWriteCloser
	dnode *dnode.DNode

	req  dnode.Message
	resp dnode.Message

	resultCallback  dnode.Callback
	methodWithID    bool
	closed          bool
	connectedClient *client
	kite            *Kite
}

func (d *DnodeClientCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	fmt.Println("Dnode WriteRequest")
	return d.enc.Encode(&d.req)
}

func (d *DnodeClientCodec) ReadResponseHeader(r *rpc.Response) error {
	fmt.Println("Dnode ReadResponseHeader")

	if err := d.dec.Decode(&d.resp); err != nil {
		return err

	}
	return nil
}

func (d *DnodeClientCodec) ReadResponseBody(x interface{}) error {
	fmt.Println("Dnode ReadResponseBody")
	return nil
}

func (d *DnodeClientCodec) Close() error {
	fmt.Println("Dnode ClientClose")
	return d.rwc.Close()
}

type DnodeServerCodec struct {
	dec             *json.Decoder
	enc             *json.Encoder
	rwc             io.ReadWriteCloser
	dnode           *dnode.DNode
	req             dnode.Message
	resultCallback  dnode.Callback
	methodWithID    bool
	closed          bool
	connectedClient *client
	kite            *Kite
}

func NewDnodeServerCodec(kite *Kite, conn io.ReadWriteCloser) rpc.ServerCodec {
	return &DnodeServerCodec{
		rwc:   conn,
		dec:   json.NewDecoder(conn),
		enc:   json.NewEncoder(conn),
		dnode: dnode.New(),
		kite:  kite,
	}
}

func (d *DnodeServerCodec) Send(method interface{}, arguments ...interface{}) {
	callbacks := make(map[string]([]string))
	d.dnode.CollectCallbacks(arguments, make([]string, 0), callbacks)

	rawArgs, err := json.Marshal(arguments)
	if err != nil {
		fmt.Printf("collect json unmarshal %+v\n", err)
	}

	message := dnode.Message{
		Method:    method,
		Arguments: &dnode.Partial{Raw: rawArgs},
		Links:     []string{},
		Callbacks: callbacks,
	}

	err = d.enc.Encode(message)
	if err != nil {
		fmt.Printf("encode err %+v\n", err)
	}
}

func (d *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	// reset values
	d.req = dnode.Message{}
	d.methodWithID = false

	// unmarshall incoming data to our dnode.Message struct
	err := d.dec.Decode(&d.req)
	if err != nil {
		return err
	}

	// if d.req.Method.(string) == "ping" {
	// 	return nil
	// }

	// for debugging: m -> c.req and m.Arguments -> c.req.Arguments
	// fmt.Printf("[received] <- %+v %+v\n", c.req.Method, string(c.req.Arguments.Raw))

	for id, path := range d.req.Callbacks {
		methodId, err := strconv.Atoi(id)
		if err != nil {
			fmt.Println("WARNING: callback id should be an INTEGER: '%s', '%s'", id, path)
			continue
		}

		callback := dnode.Callback(func(args ...interface{}) {
			if d.closed {
				return
			}

			d.Send(methodId, args...)
		})

		d.req.Arguments.Callbacks = append(d.req.Arguments.Callbacks,
			dnode.CallbackSpec{path, callback})
	}

	// received a dnode message with an method of type integer (ID), thus call our
	// stored callback that is related with this incoming ID.
	if index, err := strconv.Atoi(fmt.Sprint(d.req.Method)); err == nil {
		d.methodWithID = true

		// args can be zero or more
		args, err := d.req.Arguments.Array()
		if err != nil {
			fmt.Printf(" 1 err \n", err)
			return err
		}

		if index < 0 || index >= len(d.dnode.Callbacks) {
			return nil
		}

		callArgs := make([]reflect.Value, len(args))
		for i, v := range args {
			callArgs[i] = reflect.ValueOf(v)
		}

		d.dnode.Callbacks[index].Call(callArgs)
		return nil
	}

	// fmt.Println(d.kite.Methods)
	method, ok := d.kite.Methods[d.req.Method.(string)]
	if !ok {
		return fmt.Errorf("method %s is not registered", d.req.Method)
	}

	r.ServiceMethod = method

	// This is not used, we use our internal sequence store that is used inside
	// the dnode package, we
	// r.Seq = 0

	return nil
}

func (d *DnodeServerCodec) ReadRequestBody(body interface{}) error {
	if d.methodWithID {
		return nil
	}

	// args  is of type *dnode.Partial
	var partials []*dnode.Partial
	err := d.req.Arguments.Unmarshal(&partials)
	if err != nil {
		return err
	}

	var options struct {
		Token           string `json:"token"`
		Kitename        string
		Username        string
		VmName          string
		CorrelationName string `json:"correlationName"`
		WithArgs        *dnode.Partial
	}

	err = partials[0].Unmarshal(&options)
	if err != nil {
		return err
	}

	var resultCallback dnode.Callback
	err = partials[1].Unmarshal(&resultCallback)
	if err != nil {
		return err
	}
	d.resultCallback = resultCallback

	if body == nil {
		return nil
	}

	a := body.(*protocol.KiteDnodeRequest)
	a.Args = options.WithArgs
	a.Token = options.Token
	a.Username = options.Username
	a.Hostname = options.CorrelationName

	if d.connectedClient == nil {
		addr := d.rwc.(*websocket.Conn).Request().RemoteAddr
		ct := d.kite.Clients.Get(&client{Addr: addr})
		if ct != nil {
			ct.Username = a.Username
			d.kite.Clients.Add(ct)
			d.connectedClient = ct
		}
	}

	// Return when kontrol is not enabled
	if !d.kite.KontrolEnabled {
		return nil
	}

	if permissions.Has(a.Token) {
		fmt.Printf("[%s] allowed token (cached) '%s'\n", d.rwc.(*websocket.Conn).Request().RemoteAddr, a.Token)
		return nil
	}

	m := protocol.Request{
		Base: protocol.Base{
			Username: a.Username,
			Token:    a.Token,
			Kitename: d.kite.Kitename + "/" + d.kite.Username,
		},
		Action: "getPermission",
	}
	fmt.Printf("asking kontrol if token '%s' from %s is valid\n", a.Token, a.Username)

	msg, _ := json.Marshal(&m)
	result := d.kite.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	json.Unmarshal(result, &resp)

	switch resp.Result {
	case protocol.AllowKite:
		if a.Token != resp.Token.ID {
			return errors.New("token is invalid")
		}
		permissions.Add(a.Token) // can be changed in the future, for now cache the token

		// get underlying websocket connection and update our clients with the
		// request data. that means remove it from the buffer list(bufClients) and
		// add it to the registered user list (clients).
		// be aware that this method is called only when a RPC call is made, that
		// means this is not called when a connection is established
		a.Username = resp.Token.Username
		if a.Username != "" {
			ws := d.rwc.(*websocket.Conn)
			addr := ws.Request().RemoteAddr
			ct := d.kite.Clients.Get(&client{Addr: addr})
			if ct != nil {
				ct.Username = a.Username
				d.kite.Clients.Add(ct)
				d.connectedClient = ct
			}
		}

		fmt.Printf("[%s] allowed token '%s'\n", d.rwc.(*websocket.Conn).Request().RemoteAddr, a.Token)
		return nil
	case protocol.PermitKite:
		fmt.Printf("denied token '%s'\n", a.Token)
		return errors.New("no permission to run")
	default:
		return errors.New("got a nonstandart response")
	}

	return nil
}

func (d *DnodeServerCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	if d.methodWithID {
		// net/rpc is complaining when we exit, with an error like:
		// "rpc: service/method request ill-formed:", however this is OK. No
		// need to worry.
		return nil
	}

	if r.Error != "" {
		d.resultCallback(CreateErrorObject(fmt.Errorf(r.Error)))
		return nil
	}

	fmt.Println("method called:", r.ServiceMethod)

	d.resultCallback(nil, body)
	return nil
}

func (d *DnodeServerCodec) Close() error {
	d.closed = true

	if d.connectedClient != nil {
		fmt.Printf("[%s] client disconnected \n", d.connectedClient.Addr)
		d.kite.Clients.Remove(&client{Addr: d.connectedClient.Addr})
	}

	return d.rwc.Close()
}

// Got from kite package
type ErrorObject struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func CreateErrorObject(err error) *ErrorObject {
	return &ErrorObject{Name: reflect.TypeOf(err).Elem().Name(), Message: err.Error()}
}
