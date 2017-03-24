package sockjsclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/koding/kite/config"
	"github.com/koding/kite/utils"

	"github.com/igm/sockjs-go/sockjs"
)

// ErrPollTimeout is returned when reading first byte of the http response
// body has timed out.
//
// It is an fatal error when waiting for the session open frame ('o').
//
// After the session is opened, the error makes the poller retry polling.
var ErrPollTimeout = errors.New("polling on XHR response has timed out")

var errAborted = errors.New("session aborted by server")

// XHRSession implements sockjs.Session with XHR transport.
type XHRSession struct {
	mu sync.Mutex

	client     *http.Client
	timeout    time.Duration
	sessionURL string
	sessionID  string
	messages   []string
	abort      chan struct{}
	req        *http.Request
	state      sockjs.SessionState
}

var _ sockjs.Session = (*XHRSession)(nil)

// DialXHR establishes a SockJS session over a XHR connection.
//
// Requires cfg.XHR to be a valid client.
func DialXHR(uri string, cfg *config.Config) (*XHRSession, error) {
	// following /server_id/session_id should always be the same for every session
	serverID := threeDigits()
	sessionID := utils.RandomString(20)
	sessionURL := uri + "/" + serverID + "/" + sessionID

	// start the initial session handshake
	sessionResp, err := cfg.XHR.Post(sessionURL+"/xhr", "text/plain", nil)
	if err != nil {
		return nil, err
	}
	defer sessionResp.Body.Close()

	if sessionResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Starting new session failed. Want: %d Got: %d",
			http.StatusOK, sessionResp.StatusCode)
	}

	frame, err := newFrameReader(sessionResp.Body, cfg.Timeout).ReadByte()
	if err != nil {
		return nil, err
	}

	if frame != 'o' {
		return nil, fmt.Errorf("can't start session, invalid frame: %s", string(frame))
	}

	return &XHRSession{
		client:     cfg.XHR,
		timeout:    cfg.Timeout,
		sessionID:  sessionID,
		sessionURL: sessionURL,
		state:      sockjs.SessionActive,
		abort:      make(chan struct{}, 1),
	}, nil
}

// NewXHRSession returns a new XHRSession, a SockJS client which supports xhr-polling:
//
//   http://sockjs.github.io/sockjs-protocol/sockjs-protocol-0.3.3.html#section-74
//
// Deprecated: Use DialXHR instead.
func NewXHRSession(opts *DialOptions) (*XHRSession, error) {
	cfg := config.New()
	cfg.XHR = opts.client()

	return DialXHR(opts.BaseURL, cfg)
}

func (x *XHRSession) ID() string {
	return x.sessionID
}

func (x *XHRSession) Recv() (string, error) {
	type requestCanceler interface {
		CancelRequest(*http.Request)
	}

	// Return previously received messages if there is any.
	if len(x.messages) > 0 {
		msg := x.messages[0]
		x.messages = x.messages[1:]
		return msg, nil
	}

	if x.isClosed() {
		return "", ErrSessionClosed
	}

	// start to poll from the server until we receive something
	for {
		req, err := http.NewRequest("POST", x.sessionURL+"/xhr", nil)
		if err != nil {
			return "", errors.New("invalid session url: " + err.Error())
		}

		req.Header.Set("Content-Type", "text/plain")

		select {
		case <-x.abort:
			if cn, ok := x.client.Transport.(requestCanceler); ok {
				cn.CancelRequest(req)
			}

			return "", &ErrSession{
				Type:  config.XHRPolling,
				State: sockjs.SessionClosed,
				Err:   errAborted,
			}
		case res := <-x.do(req):
			if res.Error != nil {
				return "", fmt.Errorf("Receiving data failed: %s", res.Error)
			}

			msg, ok, err := x.handleResp(res.Response)
			if err != nil {
				return "", err
			}

			if ok {
				continue
			}

			return msg, nil
		}
	}
}

func (x *XHRSession) setState(state sockjs.SessionState) {
	x.mu.Lock()
	x.state = state
	x.mu.Unlock()
}

// GetSessionState gives state of the session.
func (x *XHRSession) GetSessionState() sockjs.SessionState {
	x.mu.Lock()
	defer x.mu.Unlock()

	return x.state
}

// Request implements the sockjs.Session interface.
func (x *XHRSession) Request() *http.Request {
	return x.req
}

func (x *XHRSession) handleResp(resp *http.Response) (msg string, again bool, err error) {
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		x.Close(3000, "session not found") // invalidate session - see details: sockjs/sockjs-client#66

		return "", false, &ErrSession{
			Type:  config.XHRPolling,
			State: sockjs.SessionClosed,
			Err:   errors.New("session does not exist: " + x.sessionID),
		}
	default:
		return "", false, fmt.Errorf("Receiving data failed. Want: 200 Got: %d", resp.StatusCode)
	}

	fr := newFrameReader(resp.Body, x.timeout)

	frame, err := fr.ReadByte()
	if err == ErrPollTimeout {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}

	switch frame {
	case 'o':
		x.setState(sockjs.SessionActive)

		return "", true, nil
	case 'm':
		var message string
		if err := json.NewDecoder(fr).Decode(&message); err != nil {
			return "", false, err
		}

		if message == "" {
			return "", false, errors.New("unexpected empty message")
		}

		x.messages = append(x.messages, message)

		message, x.messages = x.messages[0], x.messages[1:]

		return message, false, nil
	case 'a':
		// received an array of messages
		var messages []string
		if err := json.NewDecoder(fr).Decode(&messages); err != nil {
			return "", false, err
		}

		x.messages = append(x.messages, messages...)

		if len(x.messages) == 0 {
			return "", false, errors.New("no message")
		}

		// Return first message in slice, and remove it from the slice, so
		// next time the others will be picked
		msg := x.messages[0]
		x.messages = x.messages[1:]

		return msg, false, nil
	case 'h':
		return "", true, nil
	case 'c':
		var code int
		var reason string
		var frame = []interface{}{&code, &reason}

		_ = json.NewDecoder(fr).Decode(&frame)

		x.setState(sockjs.SessionClosed)

		return "", false, &ErrSession{
			Type:  config.XHRPolling,
			State: sockjs.SessionClosed,
			Err:   fmt.Errorf("closed by server: code=%d, reason=%q", code, reason),
		}
	default:
		return "", false, errors.New("invalid frame type")
	}
}

func (x *XHRSession) Send(frame string) error {
	if x.isClosed() {
		return ErrSessionClosed
	}

	body, err := json.Marshal([]string{frame})
	if err != nil {
		return err
	}

	resp, err := x.client.Post(x.sessionURL+"/xhr_send", "text/plain", bytes.NewReader(body))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		x.Close(3000, "session not found") // invalidate session - see details: sockjs/sockjs-client#66

		return &ErrSession{
			Type:  config.XHRPolling,
			State: sockjs.SessionClosed,
			Err:   errors.New("session does not exist: " + x.sessionID),
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Sending data failed. Want: %d Got: %d", http.StatusOK, resp.StatusCode)
	}

	return nil
}

func (x *XHRSession) Close(status uint32, reason string) error {
	x.setState(sockjs.SessionClosed)

	select {
	case x.abort <- struct{}{}:
	default:
	}

	return nil
}

func (x *XHRSession) isClosed() bool {
	return x.GetSessionState() == sockjs.SessionClosed
}

type doResult struct {
	Response *http.Response
	Error    error
}

func (x *XHRSession) do(req *http.Request) <-chan doResult {
	ch := make(chan doResult, 1)

	go func() {
		var res doResult
		res.Response, res.Error = x.client.Do(req)
		ch <- res
	}()

	return ch
}

type frameReader struct {
	r       *bufio.Reader
	timeout time.Duration

	once  sync.Once
	frame byte
	err   error
}

func newFrameReader(r io.Reader, timeout time.Duration) *frameReader {
	return &frameReader{
		r:       bufio.NewReader(r),
		timeout: timeout,
	}
}

func (fr *frameReader) Read(p []byte) (int, error) {
	fr.once.Do(fr.readFrame)

	if fr.err != nil {
		return 0, fr.err
	}

	var n int

	if fr.frame != 0 {
		p[0], p = fr.frame, p[1:]
		fr.frame, n = 0, 1
	}

	m, err := fr.r.Read(p)
	return n + m, err
}

func (fr *frameReader) ReadByte() (byte, error) {
	fr.once.Do(fr.readFrame)

	if fr.err != nil {
		return 0, fr.err
	}

	if fr.frame != 0 {
		c := fr.frame
		fr.frame = 0
		return c, nil
	}

	return fr.r.ReadByte()
}

func (fr *frameReader) readFrame() {
	type result struct {
		c   byte
		err error
	}
	done := make(chan result, 1)

	go func() {
		c, err := fr.r.ReadByte()
		done <- result{c, err}
	}()

	select {
	case res := <-done:
		fr.frame, fr.err = res.c, res.err
	case <-time.After(fr.timeout):
		fr.err = ErrPollTimeout
	}
}
