package kite

import (
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/cache"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/kitekey"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/sockjsclient"
	"github.com/koding/kite/utils"
)

// Request contains information about the incoming request.
type Request struct {
	// ID is an unique string, which may be used for tracing the request.
	ID string

	// Method defines the method name which is invoked by the incoming request.
	Method string

	// Username defines the username which the incoming request is bound to.
	// This is authenticated and validated if authentication is enabled.
	Username string

	// Args defines the incoming arguments for the given method.
	Args *dnode.Partial

	// LocalKite defines a context for the local kite.
	LocalKite *Kite

	// Client defines a context for the remote kite.
	Client *Client

	// Auth stores the authentication information for the incoming request and
	// the type of authentication. This is not used when authentication is disabled.
	Auth *Auth

	// Context holds a context that used by the current ServeKite handler. Any
	// items added to the Context can be fetched from other handlers in the
	// chain. This is useful with PreHandle and PostHandle handlers to pass
	// data between handlers.
	Context cache.Cache
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
			kiteErr := createError(request, r)
			c.LocalKite.Log.Error(kiteErr.Error()) // let's log it too :)
			callFunc(nil, kiteErr)
		}
	}()

	// The request that will be constructed from incoming dnode message.
	request, callFunc = c.newRequest(method.name, args)
	if method.authenticate {
		if err := request.authenticate(); err != nil {
			callFunc(nil, createError(request, err))
			return
		}
	} else {
		// if not validated accept any username it sends, also useful for test
		// cases.
		request.Username = request.Client.Kite.Username
	}

	method.mu.Lock()
	if !method.initialized {
		method.preHandlers = append(method.preHandlers, c.LocalKite.preHandlers...)
		method.postHandlers = append(method.postHandlers, c.LocalKite.postHandlers...)
		method.finalFuncs = append(method.finalFuncs, c.LocalKite.finalFuncs...)
		method.initialized = true
	}
	method.mu.Unlock()

	// check if any throttling is enabled and then check token's available.
	// Tokens are filled per frequency of the initial bucket, so every request
	// is going to take one token from the bucket. If many requests come in (in
	// span time larger than the bucket's frequency), there will be no token's
	// available more so it will return a zero.
	if method.bucket != nil && method.bucket.TakeAvailable(1) == 0 {
		callFunc(nil, &Error{
			Type:      "requestLimitError",
			Message:   "The maximum request rate is exceeded.",
			RequestID: request.ID,
		})
		return
	}

	// Call the handler functions.
	result, err := method.ServeKite(request)

	callFunc(result, createError(request, err))
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
			c.m.Lock()
			c.Kite = options.Kite
			c.m.Unlock()
			c.LocalKite.callOnFirstRequestHandlers(c)
		})
	}

	request := &Request{
		ID:        utils.RandomString(16),
		Method:    method,
		Args:      options.WithArgs,
		LocalKite: c.LocalKite,
		Client:    c,
		Auth:      options.Auth,
		Context:   cache.NewMemory(),
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
	// Trust the Kite if we have initiated the connection.  Following casts
	// means, session is opened by the client.
	if _, ok := r.Client.session.(*sockjsclient.WebsocketSession); ok {
		return nil
	}

	if _, ok := r.Client.session.(*sockjsclient.XHRSession); ok {
		return nil
	}

	if r.Auth == nil {
		return &Error{
			Type:    "authenticationError",
			Message: "No authentication information is provided",
		}
	}

	// Select authenticator function.
	f := r.LocalKite.Authenticators[r.Auth.Type]
	if f == nil {
		return &Error{
			Type:    "authenticationError",
			Message: fmt.Sprintf("Unknown authentication type: %s", r.Auth.Type),
		}
	}

	// Call authenticator function. It sets the Request.Username field.
	err := f(r)
	if err != nil {
		return &Error{
			Type:    "authenticationError",
			Message: fmt.Sprintf("%s: %s", r.Auth.Type, err),
		}
	}

	// Replace username of the remote Kite with the username that client send
	// us. This prevents a Kite to impersonate someone else's Kite.
	r.Client.SetUsername(r.Username)
	return nil
}

// AuthenticateFromToken is the default Authenticator for Kite.
func (k *Kite) AuthenticateFromToken(r *Request) error {
	k.verifyOnce.Do(k.verifyInit)

	token, err := jwt.ParseWithClaims(r.Auth.Key, &kitekey.KiteClaims{}, r.LocalKite.RSAKey)

	if e, ok := err.(*jwt.ValidationError); ok {
		// Translate public key mismatch errors to token-is-expired one.
		// This is to signal remote client the key pairs have been
		// updated on kontrol and it should invalidate all tokens.
		if (e.Errors & jwt.ValidationErrorSignatureInvalid) != 0 {
			return errors.New("token is expired")
		}
	}

	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("Invalid signature in token")
	}

	claims, ok := token.Claims.(*kitekey.KiteClaims)
	if !ok {
		return errors.New("token does not have valid claims")
	}

	if claims.Audience == "" {
		return errors.New("token has no audience")
	}

	if claims.Subject == "" {
		return errors.New("token has no username")
	}

	// check if we have an audience and it matches our own signature
	if err := k.verifyAudienceFunc(k.Kite(), claims.Audience); err != nil {
		return err
	}

	// We don't check for exp and nbf claims here because jwt-go package
	// already checks them.

	// replace the requester username so we reflect the validated
	r.Username = claims.Subject

	return nil
}

// AuthenticateFromKiteKey authenticates user from kite key.
func (k *Kite) AuthenticateFromKiteKey(r *Request) error {
	claims := &kitekey.KiteClaims{}

	token, err := jwt.ParseWithClaims(r.Auth.Key, claims, k.verify)
	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("Invalid signature in kite key")
	}

	if claims.Subject == "" {
		return errors.New("token has no username")
	}

	r.Username = claims.Subject

	return nil
}

// AuthenticateSimpleKiteKey authenticates user from the given kite key and
// returns the authenticated username. It's the same as AuthenticateFromKiteKey
// but can be used without the need for a *kite.Request.
func (k *Kite) AuthenticateSimpleKiteKey(key string) (string, error) {
	claims := &kitekey.KiteClaims{}

	token, err := jwt.ParseWithClaims(key, claims, k.verify)
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", errors.New("Invalid signature in token")
	}

	if claims.Subject == "" {
		return "", errors.New("token has no username")
	}

	return claims.Subject, nil
}

func (k *Kite) verifyInit() {
	k.configMu.Lock()
	defer k.configMu.Unlock()

	k.verifyFunc = k.Config.VerifyFunc

	if k.verifyFunc == nil {
		k.verifyFunc = k.selfVerify
	}

	k.verifyAudienceFunc = k.Config.VerifyAudienceFunc

	if k.verifyAudienceFunc == nil {
		k.verifyAudienceFunc = k.verifyAudience
	}

	ttl := k.Config.VerifyTTL

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	if ttl > 0 {
		k.mu.Lock()
		k.verifyCache = cache.NewMemoryWithTTL(ttl)
		k.mu.Unlock()

		k.verifyCache.StartGC(ttl / 2)
	}

	key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(k.Config.KontrolKey))
	if err != nil {
		k.Log.Error("unable to init kontrol key: %s", err)

		return
	}

	k.kontrolKey = key
}

func (k *Kite) selfVerify(pub string) error {
	k.configMu.RLock()
	ourKey := k.Config.KontrolKey
	k.configMu.RUnlock()

	if pub != ourKey {
		return ErrKeyNotTrusted
	}

	return nil
}

func (k *Kite) verify(token *jwt.Token) (interface{}, error) {
	k.verifyOnce.Do(k.verifyInit)

	key := token.Claims.(*kitekey.KiteClaims).KontrolKey
	if key == "" {
		return nil, errors.New("no kontrol key found")
	}

	rsaKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(key))
	if err != nil {
		return nil, err
	}

	switch {
	case k.verifyCache != nil:
		v, err := k.verifyCache.Get(key)
		if err != nil {
			break
		}

		if !v.(bool) {
			return nil, errors.New("invalid kontrol key found")
		}

		return rsaKey, nil
	}

	if err := k.verifyFunc(key); err != nil {
		if err == ErrKeyNotTrusted {
			k.verifyCache.Set(key, false)
		}

		// signal old token to somewhere else (GetKiteKey and alike)

		return nil, err
	}

	k.verifyCache.Set(key, true)

	return rsaKey, nil
}

func (k *Kite) verifyAudience(kite *protocol.Kite, audience string) error {
	switch audience {
	case "/":
		// The root audience is like superuser - it has access to everything.
		return nil
	case "":
		return errors.New("invalid empty audience")
	}

	aud, err := protocol.KiteFromString(audience)
	if err != nil {
		return fmt.Errorf("invalid audience: %s (%s)", err, audience)
	}

	// We verify the Username / Environment / Name matches the kite.
	// Empty field (except username) is like wildcard - it matches all values.

	if kite.Username != aud.Username {
		return fmt.Errorf("audience: username %q not allowed (%s)", aud.Username, audience)
	}

	if kite.Environment != aud.Environment && aud.Environment != "" {
		return fmt.Errorf("audience: environment %q not allowed (%s)", aud.Environment, audience)
	}

	if kite.Name != aud.Name && aud.Name != "" {
		return fmt.Errorf("audience: kite %q not allowed (%s)", aud.Name, audience)
	}

	return nil
}
