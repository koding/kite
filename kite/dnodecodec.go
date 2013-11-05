// Dnode protocol for net/rpc
package kite

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
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
	dec            *json.Decoder
	enc            *json.Encoder
	rwc            io.ReadWriteCloser
	dnode          *dnode.DNode
	req            dnode.Message
	resultCallback dnode.Callback
	methodWithID   bool
	closed         bool
	kite           *Kite

	// connectedClient is setup once for every client.
	connectedClient *client
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
			fmt.Printf("WARNING: callback id should be an INTEGER: '%s', '%s'\n", id, path)
			continue
		}

		callback := dnode.Callback(func(args ...interface{}) {
			if d.closed {
				return
			}

			d.Send(methodId, args...)
		})

		d.req.Arguments.Callbacks = append(d.req.Arguments.Callbacks,
			dnode.CallbackSpec{
				Path:     path,
				Callback: callback,
			})
	}

	// received a dnode message with an method of type integer (ID), thus call our
	// stored callback that is related with this incoming ID.
	if index, err := strconv.Atoi(fmt.Sprint(d.req.Method)); err == nil {
		d.methodWithID = true

		// args can be zero or more
		args, err := d.req.Arguments.Array()
		if err != nil {
			fmt.Printf("1 err: %s\n", err)
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

	// args is of type *dnode.Partial
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

	if options.Token == "" {
		return errors.New("Token is not sent")
	}

	if body == nil {
		return nil
	}

	req := body.(*protocol.KiteDnodeRequest)
	req.Args = options.WithArgs
	req.Username = options.Username
	req.Hostname = options.CorrelationName

	// Return when kontrol is not enabled
	if !d.kite.KontrolEnabled {
		return nil
	}

	// Ignoring error because the key will be used in decrypt below.
	key, _ := kodingkey.FromString(d.kite.KodingKey)

	// DecryptString will fail if the key is not valid.
	tkn, err := token.DecryptString(options.Token, key)
	if err != nil {
		return errors.New("Invalid token")
	}

	if !tkn.IsValid(d.kite.ID) {
		fmt.Printf("Invalid token '%s'\n", options.Token)
		return errors.New("Invalid token")
	}

	req.Username = tkn.Username
	d.UpdateClient(tkn.Username)

	fmt.Printf("[%s] allowed token for: '%s'\n", d.ClientAddr(), req.Username)
	return nil
}

// update our clients map with the request data (for now only with username).
// Be aware that this method is called only when a RPC call is made, that
// means this is not called when a connection is established.
func (d *DnodeServerCodec) UpdateClient(username string) {
	if d.connectedClient != nil {
		return // we already got every detail
	}

	client := d.kite.clients.GetClient(d.ClientAddr())
	if client == nil {
		return

	}

	if username == "" {
		return
	}

	client.Username = username
	d.connectedClient = client

	d.kite.clients.AddAddresses(username, d.ClientAddr())

	// update username within client struct
	d.kite.clients.AddClient(d.ClientAddr(), client)

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
	fmt.Printf("[%s] disconnected \n", d.ClientAddr())
	d.closed = true
	d.CallOnDisconnectFuncs()

	return d.rwc.Close()
}

func (d *DnodeServerCodec) CallOnDisconnectFuncs() {
	if d.connectedClient == nil {
		return
	}

	client := d.kite.clients.GetClient(d.ClientAddr())
	if client == nil {
		return
	}

	d.kite.clients.RemoveAddresses(client.Username, d.ClientAddr())
	addrs := d.kite.clients.GetAddresses(client.Username)

	if len(addrs) > 0 {
		return
	}

	for _, f := range client.onDisconnect {
		f()
	}

	d.kite.clients.RemoveClient(d.ClientAddr())
}

// Addr returns the connected clients addres
func (d *DnodeServerCodec) ClientAddr() string {
	return d.rwc.(*websocket.Conn).Request().RemoteAddr
}

// Got from kite package
type ErrorObject struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func CreateErrorObject(err error) *ErrorObject {
	return &ErrorObject{Name: reflect.TypeOf(err).Elem().Name(), Message: err.Error()}
}
