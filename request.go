package kite

import (
	"errors"
	"fmt"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/dnode/rpc"
	"github.com/koding/kite/kitekey"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/dgrijalva/jwt-go"
)

// runMethod is called when a method is received from remote Kite.
func runMethod(method string, handlerFunc reflect.Value, args dnode.Arguments, tr dnode.Transport) {
	var (
		// The request that will be constructed from incoming dnode message.
		request *Request

		// Will hold the return values from handler func.
		values []reflect.Value

		// First value to the response.
		result interface{}

		// Second value to the response.
		kiteErr *Error

		// Will send the response when called.
		callback dnode.Function
	)

	kite := tr.Properties()["localKite"].(*Kite)

	// Send result if "responseCallback" exists in the request.
	defer func() {
		if callback == nil {
			return
		}

		// Only argument to the callback.
		response := callbackArg{
			Result: result,
			Error:  errorForSending(kiteErr),
		}

		// Call response callback function.
		if err := callback(response); err != nil {
			kite.Log.Error(err.Error())
		}
	}()

	// Recover dnode argument errors. The caller can use functions like
	// MustString(), MustSlice()... without the fear of panic.
	defer kite.recoverError(&kiteErr)()

	request, callback = kite.parseRequest(method, args, tr)

	if !kite.disableAuthenticate {
		kiteErr = request.authenticate()
		if kiteErr != nil {
			return
		}
	}

	// Call the handler function.
	callArgs := []reflect.Value{reflect.ValueOf(request)}
	values = handlerFunc.Call(callArgs)

	result = values[0].Interface()

	if err := values[1].Interface(); err != nil {
		panic(err) // This will be recoverd from kite.recoverError() above.
	}
}

// Error is the type of the kite related errors returned from kite package.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e Error) Error() string {
	return fmt.Sprintf("kite error %s - %s", e.Type, e.Message)
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

// recoverError returns a function which recovers the error and sets to the
// given argument as kite.Error.
//
// TODO: change that it doesn't use a pointer of pointer,
// a simpler and cleaner solution would work in the future,
func (k *Kite) recoverError(kiteErr **Error) func() {
	return func() {
		r := recover()
		if r == nil {
			return
		}

		switch err := r.(type) {
		case *Error:
			*kiteErr = err
		case *dnode.ArgumentError:
			*kiteErr = &Error{"argumentError", err.Error()}
		default:
			*kiteErr = &Error{"genericError", fmt.Sprint(r)}
			debug.PrintStack()
		}

		k.Log.Warning("Error in received message %s", (*kiteErr).Error())

	}
}

// HandlerFunc is the type of the handlers registered to Kite.
// The returned result must be Marshalable with json package.
type HandlerFunc func(*Request) (result interface{}, err error)

// HandleFunc registers a handler to run when a method call is received from a Kite.
func (k *Kite) HandleFunc(method string, handler HandlerFunc) {
	k.server.HandleFunc(method, handler)
}

// Request contains information about the incoming request.
type Request struct {
	Method         string
	Args           dnode.Arguments
	LocalKite      *Kite
	RemoteKite     *RemoteKite
	Username       string
	Authentication Authentication
	RemoteAddr     string
}

// Wrap your function with Callback to send it as an argument to a RemoteKite.
type Callback func(r *Request)

func (c Callback) MarshalJSON() ([]byte, error) {
	return []byte(`"[Function]"`), nil
}

// runCallback is called when a callback method call is received from remote Kite.
func runCallback(method string, handlerFunc reflect.Value, args dnode.Arguments, tr dnode.Transport) {
	kite := tr.Properties()["localKite"].(*Kite)

	kiteErr := new(Error)               // Not used. For recovering the error.
	defer kite.recoverError(&kiteErr)() // Do not panic no matter what.

	request, _ := kite.parseRequest(method, args, tr)

	// Call the callback function.
	callArgs := []reflect.Value{reflect.ValueOf(request)}
	handlerFunc.Call(callArgs) // No return value from callback function.
}

// parseRequest is used to read a dnode message.
// It is called when a method or callback is received to parse the message.
func (k *Kite) parseRequest(method string, arguments dnode.Arguments, tr dnode.Transport) (*Request, dnode.Function) {
	// Parse dnode method arguments: [options]
	var options callOptions
	arguments.One().MustUnmarshal(&options)

	// Properties about the client...
	properties := tr.Properties()

	// Create a new RemoteKite instance to pass it to the handler, so
	// the handler can call methods on the other site on the same connection.
	var remoteKite *RemoteKite
	if properties["remoteKite"] == nil {
		// Do not create a new RemoteKite on every request,
		// cache it in Transport.Properties().
		client := tr.(*rpc.Client) // We only have a dnode/rpc.Client for now.
		remoteKite = k.newRemoteKiteWithClient(options.Kite, options.Authentication, client)
		properties["remoteKite"] = remoteKite

		// Notify Kite.OnConnect handlers.
		k.notifyRemoteKiteConnected(remoteKite)
	} else {
		remoteKite = properties["remoteKite"].(*RemoteKite)

		// Need to update URL in case of a change. For example, the remote kite
		// may disconnect from a proxy kite and registers to different proxy.
		remoteKite.URL = options.Kite.URL
	}

	request := &Request{
		Method:         method,
		Args:           options.WithArgs,
		LocalKite:      k,
		RemoteKite:     remoteKite,
		RemoteAddr:     tr.RemoteAddr(),
		Authentication: options.Authentication,
	}

	return request, options.ResponseCallback
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
	token, err := jwt.Parse(r.Authentication.Key, r.LocalKite.getRSAKey)
	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("Invalid signature in token")
	}

	if audience, ok := token.Claims["aud"].(string); !ok || !strings.HasPrefix(k.Kite.Key(), audience) {
		return fmt.Errorf("Invalid audience in token: %s", audience)
	}

	// We don't check for exp and nbf claims here because jwt-go package already checks them.

	if username, ok := token.Claims["sub"].(string); !ok {
		return errors.New("Username is not present in token")
	} else {
		r.Username = username
	}

	return nil
}

// AuthenticateFromKiteKey authenticates user from Koding Key.
// Kontrol makes requests with a Koding Key.
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
