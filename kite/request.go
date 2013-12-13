package kite

import (
	"errors"
	"fmt"
	"koding/newkite/dnode"
	"koding/newkite/dnode/rpc"
	"koding/newkite/kodingkey"
	"koding/newkite/token"
	"reflect"
)

// runMethod is called when a method is received from remote.
func runMethod(method string, handlerFunc reflect.Value, args dnode.Arguments, tr dnode.Transport) {
	var (
		// Will hold the return values from handler func.
		values []reflect.Value

		// First value to the response.
		result interface{}

		// Second value to the response.
		kiteError *Error

		// Will send the response when called.
		callback dnode.Function
	)

	kite := tr.Properties()["localKite"].(*Kite)

	defer func() {
		if callback == nil {
			return
		}

		// Only argument to the callback.
		response := callbackArg{
			Result: result,
			Error:  errorForSending(kiteError),
		}

		// Call response callback function.
		if err := callback(response); err != nil {
			kite.Log.Error(err.Error())
		}
	}()

	// Recover dnode argument errors.
	// The caller can use functions like MustString(), MustSlice()...
	// without the fear of panic.
	argumentError := recoverArgumentError(func() {
		var request *Request
		var err error

		request, callback, err = kite.parseRequest(method, args, tr)
		if err != nil {
			kite.Log.Notice("Did not understand request: %s", err)
			return
		}

		kiteError = request.authenticate()
		if kiteError != nil {
			return
		}

		callArgs := []reflect.Value{reflect.ValueOf(request)}
		// Call the handler.
		values = handlerFunc.Call(callArgs)
	})
	if argumentError != nil {
		kiteError = &Error{"argumentError", argumentError.Error()}
		return
	}

	result = values[0].Interface()

	errVal := values[1].Interface()
	if errVal == nil {
		return
	}

	var ok bool
	if kiteError, ok = errVal.(*Error); ok {
		return
	}

	kiteError = &Error{"genericError", errVal.(error).Error()}
}

// Error is the type of the first argument in response callback.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("Kite error: %s: %s", e.Type, e.Message)
}

// When a callback is called we always pass this as the only argument.
type callbackArg struct {
	Error  errorForSending `json:"error"`
	Result interface{}     `json:"result"`
}

// errorForSending is a for sending the error as an argument in a dnode message.
// Normally Error method of the Error struct is sent as a callback since it is
// exported. We do not want this behavior.
type errorForSending *Error

// recoverArgumentError takes a function and tries to recover a dnode.ArgumentError
// if it panics.
func recoverArgumentError(f func()) (err *dnode.ArgumentError) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		var ok bool
		if err, ok = r.(*dnode.ArgumentError); !ok {
			panic(r)
		}
	}()

	f()

	return
}

type HandlerFunc func(*Request) (result interface{}, err error)

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (k *Kite) HandleFunc(method string, handler HandlerFunc) {
	k.server.HandleFunc(method, handler)
}

type Request struct {
	Method         string
	Args           dnode.Arguments
	LocalKite      *Kite
	RemoteKite     *RemoteKite
	Username       string
	Authentication Authentication
	RemoteAddr     string
}

type Callback func(r *Request)

func (c Callback) MarshalJSON() ([]byte, error) {
	return []byte(`"[Function]"`), nil
}

// runCallback is called when a callback method call is received from remote.
func runCallback(method string, handlerFunc reflect.Value, args dnode.Arguments, tr dnode.Transport) {
	k := tr.Properties()["localKite"].(*Kite)

	recoverArgumentError(func() {
		request, _, err := k.parseRequest(method, args, tr)
		if err != nil {
			k.Log.Notice("Did not understand callback message: %s. method: %q args: %q", err, method, args)
			return
		}

		callArgs := []reflect.Value{reflect.ValueOf(request)}
		handlerFunc.Call(callArgs)
	})
}

// parseRequest is used to read a dnode message.
// It is called when a method or callback is received.
func (k *Kite) parseRequest(method string, arguments dnode.Arguments, tr dnode.Transport) (
	request *Request, responseCallback dnode.Function, err error) {

	// Parse dnode method arguments: [options]
	var options callOptions
	arguments.One().MustUnmarshal(&options)

	responseCallback = options.ResponseCallback

	// Properties about the client...
	properties := tr.Properties()

	// Create a new RemoteKite instance to pass it to the handler, so
	// the handler can call methods on the other site on the same connection.
	if properties["remoteKite"] == nil {
		// Do not create a new RemoteKite on every request,
		// cache it in Transport.Properties().
		client := tr.(*rpc.Client) // We only have a dnode/rpc.Client for now.
		remoteKite := k.newRemoteKiteWithClient(options.Kite, options.Authentication, client)
		properties["remoteKite"] = remoteKite

		// Notify Kite.OnConnect handlers.
		k.notifyRemoteKiteConnected(remoteKite)
	}

	request = &Request{
		Method:         method,
		Args:           options.WithArgs,
		LocalKite:      k,
		RemoteKite:     properties["remoteKite"].(*RemoteKite),
		RemoteAddr:     tr.RemoteAddr(),
		Username:       options.Kite.Username,
		Authentication: options.Authentication,
	}

	return
}

// authenticate tries to authenticate the user by selecting appropriate
// authenticator function.
func (r *Request) authenticate() *Error {
	// Trust the Kite if we have initiated the connection.
	// RemoteAddr() returns "" if this is an outgoing connection.
	if r.RemoteAddr == "" {
		return nil
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

	// Fix username of the remote Kite if it is invalid.
	// This prevents a Kite to impersonate someone else's Kite.
	r.RemoteKite.Kite.Username = r.Username

	return nil
}

// AuthenticateFromToken is the default Authenticator for Kite.
func (k *Kite) AuthenticateFromToken(r *Request) error {
	key, err := kodingkey.FromString(k.KodingKey)
	if err != nil {
		return fmt.Errorf("Invalid Koding Key: %s", k.KodingKey)
	}

	tkn, err := token.DecryptString(r.Authentication.Key, key)
	if err != nil {
		return fmt.Errorf("Invalid token: %s", r.Authentication.Key)
	}

	if !tkn.IsValid(k.ID) {
		return fmt.Errorf("Invalid token: %s", tkn)
	}

	r.Username = tkn.Username

	return nil
}

// AuthenticateFromToken authenticates user from Koding Key.
// Kontrol makes requests with a Koding Key.
func (k *Kite) AuthenticateFromKodingKey(r *Request) error {
	if r.Authentication.Key != k.KodingKey {
		return errors.New("Invalid Koding Key")
	}

	// Set the username if missing.
	if r.RemoteKite.Username == "" && k.Username != "" {
		r.RemoteKite.Username = k.Username
	}

	return nil
}
