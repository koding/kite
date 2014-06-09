package kite

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/sockjsclient"
)

// Request contains information about the incoming request.
type Request struct {
	Method         string
	Args           *dnode.Partial
	LocalKite      *Kite
	Client         *Client
	Username       string
	Authentication *Authentication
}

// Response is the type of the object that is returned from request handlers
// and the type of only argument that is passed to callback functions.
type Response struct {
	Error  *Error      `json:"error" dnode:"-"`
	Result interface{} `json:"result"`
}

// runMethod is called when a method is received from remote Kite.
func (c *Client) runMethod(method *Method, args *dnode.Partial) {
	var (
		callFunc func(interface{}, *Error)
		request  *Request
	)

	// Recover dnode argument errors and send them back. The caller can use
	// functions like MustString(), MustSlice()... without the fear of panic.
	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			callFunc(nil, createError(r))
		}
	}()

	// The request that will be constructed from incoming dnode message.
	request, callFunc = c.newRequest(method.name, args)
	if method.authenticate {
		if err := request.authenticate(); err != nil {
			callFunc(nil, err)
			return
		}
	} else {
		// if not valided accept any username it sends, also useful for test
		// cases.
		request.Username = request.Client.Kite.Username
	}

	// Call the handler function.
	result, err := method.handler.ServeKite(request)

	callFunc(result, createError(err))
}

// runCallback is called when a callback method call is received from remote Kite.
func (c *Client) runCallback(callback func(*dnode.Partial), args *dnode.Partial) {
	// Do not panic no matter what.
	defer func() {
		if err := recover(); err != nil {
			c.LocalKite.Log.Warning("Error in calling the callback function : %v", err)
		}
	}()

	// Call the callback function.
	callback(args)
}

// newRequest returns a new *Request from the method and arguments passed.
func (c *Client) newRequest(method string, args *dnode.Partial) (*Request, func(interface{}, *Error)) {
	// Parse dnode method arguments: [options]
	var options callOptions
	args.One().MustUnmarshal(&options)

	// Notify the handlers registered with Kite.OnFirstRequest().
	if _, ok := c.session.(*sockjsclient.WebsocketSession); !ok {
		c.firstRequestHandlersNotified.Do(func() {
			c.Kite = options.Kite
			c.LocalKite.callOnFirstRequestHandlers(c)
		})
	}

	request := &Request{
		Method:         method,
		Args:           options.WithArgs,
		LocalKite:      c.LocalKite,
		Client:         c,
		Authentication: options.Authentication,
	}

	// Call response callback function, send back our response
	callFunc := func(result interface{}, err *Error) {
		if options.ResponseCallback.Caller == nil {
			return
		}

		// Only argument to the callback.
		response := Response{
			Result: result,
			Error:  err,
		}

		if err := options.ResponseCallback.Call(response); err != nil {
			c.LocalKite.Log.Error(err.Error())
		}
	}

	return request, callFunc
}

// authenticate tries to authenticate the user by selecting appropriate
// authenticator function.
func (r *Request) authenticate() *Error {
	// Trust the Kite if we have initiated the connection.
	// Following cast means, session is opened by the client.
	if _, ok := r.Client.session.(*sockjsclient.WebsocketSession); ok {
		return nil
	}

	if r.Authentication == nil {
		return &Error{
			Type:    "authenticationError",
			Message: "No authentication information is provided",
		}
	}

	// Select authenticator function.
	f := r.LocalKite.Authenticators[r.Authentication.Type]
	if f == nil {
		return &Error{
			Type:    "authenticationError",
			Message: fmt.Sprintf("Unknown authentication type: %s", r.Authentication.Type),
		}
	}

	// Call authenticator function. It sets the Request.Username field.
	err := f(r)
	if err != nil {
		return &Error{
			Type:    "authenticationError",
			Message: err.Error(),
		}
	}

	// Replace username of the remote Kite with the username that client send
	// us. This prevents a Kite to impersonate someone else's Kite.
	r.Client.Kite.Username = r.Username
	return nil
}

// AuthenticateFromToken is the default Authenticator for Kite.
func (k *Kite) AuthenticateFromToken(r *Request) error {
	token, err := jwt.Parse(r.Authentication.Key, r.LocalKite.RSAKey)
	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("Invalid signature in token")
	}

	if audience, ok := token.Claims["aud"].(string); !ok || !strings.HasPrefix(k.Kite().String(), audience) {
		return fmt.Errorf("Invalid audience in token. \nHave: %s \nMust be a part of: %s", audience, k.Kite().String())
	}

	// We don't check for exp and nbf claims here because jwt-go package already checks them.
	if username, ok := token.Claims["sub"].(string); !ok {
		return errors.New("Username is not present in token")
	} else {
		r.Username = username
	}

	return nil
}

// AuthenticateFromKiteKey authenticates user from kite key.
func (k *Kite) AuthenticateFromKiteKey(r *Request) error {
	token, err := jwt.Parse(r.Authentication.Key, kitekey.GetKontrolKey)
	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("Invalid signature in token")
	}

	if username, ok := token.Claims["sub"].(string); !ok {
		return errors.New("Username is not present in token")
	} else {
		r.Username = username
	}

	return nil
}
