// Dnode protocol for net/rpc
package kite

import "net/rpc"

/******************************************

Client

******************************************/

type DnodeClientCodec struct{}

func (c *DnodeClientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	return nil
}

func (c *DnodeClientCodec) ReadResponseHeader(r *rpc.Response) error {
	return nil
}

func (c *DnodeClientCodec) ReadResponseBody(x interface{}) error {
	return nil
}

func (c *DnodeClientCodec) Close() error {
	return nil
}

/******************************************

SERVER

******************************************/

type DnodeServerCodec struct{}

func (c *DnodeServerCodec) ReadRequestHeader(r *rpc.Request) error {
	return nil
}

func (c *DnodeServerCodec) ReadRequestBody(x interface{}) error {
	return nil
}

func (c *DnodeServerCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	return nil
}

func (c *DnodeServerCodec) Close() error {
	return nil
}
