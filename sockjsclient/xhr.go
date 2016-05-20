package sockjsclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sync"
)

// the implementation of New() doesn't have any error to be returned yet it
// returns, so it's totally safe to neglect the error
var cookieJar, _ = cookiejar.New(nil)

type XHRSession struct {
	mu sync.Mutex

	client     *http.Client
	sessionURL string
	sessionID  string
	messages   []string
	opened     bool
	abort      chan struct{}
}

// NewXHRSession returns a new XHRSession, a SockJS client which supports
// xhr-polling
// http://sockjs.github.io/sockjs-protocol/sockjs-protocol-0.3.3.html#section-74
func NewXHRSession(opts *DialOptions) (*XHRSession, error) {
	client := opts.client()

	// following /server_id/session_id should always be the same for every session
	serverID := threeDigits()
	sessionID := randomStringLength(20)
	sessionURL := opts.BaseURL + "/" + serverID + "/" + sessionID

	// start the initial session handshake
	sessionResp, err := client.Post(sessionURL+"/xhr", "text/plain", nil)
	if err != nil {
		return nil, err
	}
	defer sessionResp.Body.Close()

	if sessionResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Starting new session failed. Want: %d Got: %d",
			http.StatusOK, sessionResp.StatusCode)
	}

	buf := bufio.NewReader(sessionResp.Body)
	frame, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	if frame != 'o' {
		return nil, fmt.Errorf("can't start session, invalid frame: %s", frame)
	}

	return &XHRSession{
		client:     client,
		sessionID:  sessionID,
		sessionURL: sessionURL,
		opened:     true,
		abort:      make(chan struct{}, 1),
	}, nil
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

	// start to poll from the server until we receive something
	for {
		req, err := http.NewRequest("POST", x.sessionURL+"/xhr", nil)
		if err != nil {
			return "", fmt.Errorf("Receiving data failed: %s", err)
		}

		req.Header.Set("Content-Type", "text/plain")

		select {
		case <-x.abort:
			if cn, ok := x.client.Transport.(requestCanceler); ok {
				cn.CancelRequest(req)
			}

			return "", fmt.Errorf("session aborted by server")
		case res := <-x.do(req):
			resp := res.Response

			if res.Error != nil {
				return "", fmt.Errorf("Receiving data failed: %s", res.Error)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("Receiving data failed. Want: %d Got: %d",
					http.StatusOK, resp.StatusCode)
			}

			buf := bufio.NewReader(resp.Body)

			// returns an error if buffer is empty
			frame, err := buf.ReadByte()
			if err != nil {
				return "", err
			}

			switch frame {
			case 'o':
				// Abort session on second 'o' frame:
				//
				//   https://github.com/sockjs/sockjs-protocol/wiki/Connecting-to-SockJS-without-the-browser
				//
				x.mu.Lock()
				x.opened = false
				x.mu.Unlock()

				return "", errors.New("session aborted")
			case 'a':
				// received an array of messages
				var messages []string
				if err := json.NewDecoder(buf).Decode(&messages); err != nil {
					return "", err
				}

				x.messages = append(x.messages, messages...)

				if len(x.messages) == 0 {
					return "", errors.New("no message")
				}

				// Return first message in slice, and remove it from the slice, so
				// next time the others will be picked
				msg := x.messages[0]
				x.messages = x.messages[1:]

				return msg, nil
			case 'h':
				// heartbeat received
				continue
			case 'c':
				x.mu.Lock()
				x.opened = false
				x.mu.Unlock()

				return "", errors.New("session closed")
			default:
				return "", errors.New("invalid frame type")
			}
		}
	}

	return "", errors.New("FATAL: If we get here, please revisit the logic again")
}

func (x *XHRSession) Send(frame string) error {
	x.mu.Lock()
	if !x.opened {
		x.mu.Unlock()
		return errors.New("session is not opened yet")
	}
	x.mu.Unlock()

	// Need's to be JSON encoded array of string messages (SockJS protocol
	// requirement)
	message := []string{frame}
	body, err := json.Marshal(&message)
	if err != nil {
		return err
	}

	resp, err := x.client.Post(x.sessionURL+"/xhr_send", "text/plain", bytes.NewReader(body))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		x.Close(0, "") // invalidate session - see details: sockjs/sockjs-client#66

		return fmt.Errorf("XHR session does not exist: %s", x.sessionID)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Sending data failed. Want: %d Got: %d",
			http.StatusOK, resp.StatusCode)
	}

	return nil
}

func (x *XHRSession) Close(status uint32, reason string) error {
	x.mu.Lock()
	x.opened = false
	x.mu.Unlock()

	select {
	case x.abort <- struct{}{}:
	default:
	}

	return nil
}

type doResult struct {
	Response *http.Response
	Error    error
}

func (x *XHRSession) do(req *http.Request) <-chan doResult {
	ch := make(chan doResult)

	go func() {
		var res doResult
		res.Response, res.Error = x.client.Do(req)
		ch <- res
	}()

	return ch
}
