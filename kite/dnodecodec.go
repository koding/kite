// Dnode protocol for net/rpc
package kite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"koding/newkite/protocol"
	"koding/tools/dnode"
	"net/rpc"
	"reflect"
	"strconv"
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
	dec            *json.Decoder
	enc            *json.Encoder
	rwc            io.ReadWriteCloser
	dnode          *dnode.DNode
	req            dnode.Message
	resultCallback dnode.Callback
	methodWithID   bool
	kite           *Kite
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

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	// reset values
	c.req = dnode.Message{}
	c.methodWithID = false

	// unmarshall incoming data to our dnode.Message struct
	err := c.dec.Decode(&c.req)
	if err != nil {
		return err
	}

	// for debugging: m -> c.req and m.Arguments -> c.req.Arguments
	// fmt.Printf("[received] <- %+v %+v\n", c.req.Method, string(c.req.Arguments.Raw))

	for id, path := range c.req.Callbacks {
		methodId, err := strconv.Atoi(id)
		if err != nil {
			fmt.Println("WARNING: callback id should be an INTEGER: '%s', '%s'", id, path)
			continue
		}

		callback := dnode.Callback(func(args ...interface{}) {
			callbacks := make(map[string]([]string))
			c.dnode.CollectCallbacks(args, make([]string, 0), callbacks)

			rawArgs, err := json.Marshal(&args)
			if err != nil {
				fmt.Printf("collect json unmarshal %+v\n", err)
			}

			message := dnode.Message{
				Method:    methodId,
				Arguments: &dnode.Partial{Raw: rawArgs},
				Links:     []string{},
				Callbacks: callbacks,
			}

			err = c.enc.Encode(message)
			if err != nil {
				fmt.Printf("encode err %+v\n", err)
			}

			// for debugging
			// fmt.Printf("[sending] -> %+v, %+v\n", c.req.Method, message)
		})

		c.req.Arguments.Callbacks = append(c.req.Arguments.Callbacks,
			dnode.CallbackSpec{path, callback})
	}

	// received a dnode message with an method of type integer (ID), thus call our
	// stored callback that is related with this incoming ID.
	if index, err := strconv.Atoi(fmt.Sprint(c.req.Method)); err == nil {
		c.methodWithID = true

		// args can be zero or more
		args, err := c.req.Arguments.Array()
		if err != nil {
			fmt.Printf(" 1 err \n", err)
			return err
		}

		if index < 0 || index >= len(c.dnode.Callbacks) {
			return nil
		}

		callArgs := make([]reflect.Value, len(args))
		for i, v := range args {
			callArgs[i] = reflect.ValueOf(v)
		}

		fmt.Printf("[%d] callback called\n", index)
		c.dnode.Callbacks[index].Call(callArgs)
		return nil
	}

	// This will be replaced with a kite protocol interface in front of net/rpc
	// method := upperFirst(strings.Split(c.req.Method.(string), ".")[1])

	// fmt.Println(c.kite.Methods)
	method, ok := c.kite.Methods[c.req.Method.(string)]
	if !ok {
		return fmt.Errorf("method %s is not registered", c.req.Method)
	}

	r.ServiceMethod = method

	// This is not used, we use our internal sequence store that is used inside
	// the dnode package, we
	// r.Seq = 0

	return nil
}

func (c *DnodeServerCodec) ReadRequestBody(body interface{}) error {
	if c.methodWithID {
		return nil
	}

	// args  is of type *dnode.Partial
	var partials []*dnode.Partial
	err := c.req.Arguments.Unmarshal(&partials)
	if err != nil {
		return err
	}

	var options struct {
		Token    string `json:"token"`
		Kitename string
		Username string
		WithArgs *dnode.Partial
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
	c.resultCallback = resultCallback

	if body == nil {
		return nil
	}

	a := body.(*protocol.KiteDnodeRequest)
	a.Args = options.WithArgs
	a.Kitename = options.Kitename
	a.Token = options.Token
	a.Username = options.Username

	fmt.Printf("got a call request from %s with token %s", a.Kitename, a.Token)
	if permissions.Has(a.Token) {
		fmt.Printf("... already allowed to run\n")
		return nil
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

	fmt.Printf("\nasking kontrol for permission, for '%s' with token '%s': -> ", a.Kitename, a.Token)
	result := c.kite.Messenger.Send(msg)

	var resp protocol.RegisterResponse
	json.Unmarshal(result, &resp)
	fmt.Println("got result")

	if a.Token != resp.Token.ID {
		return errors.New("token is invalid")
	}

	switch resp.Result {
	case protocol.AllowKite:
		fmt.Println("... allowed to run\n")
		permissions.Add(a.Token) // can be changed in the future, for now cache the token
		return nil
	case protocol.PermitKite:
		fmt.Println("... not allowed. permission denied via Kontrol\n")
		return errors.New("no permission to run")
	default:
		return errors.New("got a nonstandart response")
	}

	return nil
}

func (c *DnodeServerCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	if c.methodWithID {
		// net/rpc is complaining when we exit, with an error like:
		// "rpc: service/method request ill-formed:", however this is OK. No
		// need to worry.
		return nil
	}

	//
	if r.Error != "" {
		c.resultCallback(CreateErrorObject(fmt.Errorf(r.Error)))
		return nil
	}

	fmt.Printf("[%s] called\n", r.ServiceMethod)
	c.resultCallback(nil, body)
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

// Got from kite package
type ErrorObject struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func CreateErrorObject(err error) *ErrorObject {
	return &ErrorObject{Name: reflect.TypeOf(err).Elem().Name(), Message: err.Error()}
}
