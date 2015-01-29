package sockjsclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

// info is returned from a SockJS Base_URL+/info path
type info struct {
	Websocket    bool     `json:"websocket"`
	CookieNeeded bool     `json:"cookie_needed"`
	Origins      []string `json:"origins"`
	Entropy      int32    `json:"entropy"`
}

func NewXHRSession(opts *DialOptions) (*XHRSession, error) {
	client := &http.Client{
		Timeout: opts.Timeout,
	}

	resp, err := client.Get(opts.BaseURL + "/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var i info
	if err := json.NewDecoder(resp.Body).Decode(&i); err != nil {
		return nil, err
	}

	// TODO: do something with the info, probably we need to put it in a more
	// high level place. Once we receive the info we can make use of WebSocket,
	// XHR, co..
	fmt.Printf("i = %+v\n", i)

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

	buf := bufio.NewReader(sessionResp.Body)
	frame, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	fmt.Printf("frame = %+v\n", frame)

	if frame != 'o' {
		return nil, fmt.Errorf("can't start session, invalid frame: %s", frame)
	}

	fmt.Println("received o")

	return &XHRSession{
		client:     client,
		sessionURL: sessionURL,
		opened:     true,
	}, nil
}

type XHRSession struct {
	client     *http.Client
	sessionURL string
	messages   []string
	opened     bool
}

func (x *XHRSession) ID() string {
	return ""
}

func (x *XHRSession) Recv() (string, error) {
	// Return previously received messages if there is any.
	if len(x.messages) > 0 {
		msg := x.messages[0]
		x.messages = x.messages[1:]
		return msg, nil
	}

	for {
		fmt.Println("recv /xhr")
		resp, err := x.client.Post(x.sessionURL+"/xhr", "text/plain", nil)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		buf := bufio.NewReader(resp.Body)

		// returns an error if buffer is empty ;)
		frame, err := buf.ReadByte()
		if err != nil {
			return "", err
		}

		fmt.Printf("received frame = %+v\n", string(frame))

		switch frame {
		case 'o':
			// session started
			x.opened = true
			continue
		case 'a':
			data, err := ioutil.ReadAll(buf)
			if err != nil {
				return "", err
			}

			fmt.Printf("string(data) = %+v\n", string(data))

			var messages []string
			err = json.Unmarshal(data, &messages)
			if err != nil {
				return "", err
			}

			fmt.Printf("messages = %+v\n", messages)

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
			// close received
		default:
			return "", errors.New("invalid frame type")
		}
	}

	return "", errors.New("FATAL: If we get here, please revisit the logic again")
}

func (x *XHRSession) Send(frame string) error {
	if !x.opened {
		return errors.New("connection is not opened yet")
	}

	fmt.Printf("sending frame = %+v\n", frame)
	message := []string{frame}
	body, err := json.Marshal(&message)
	if err != nil {
		return err
	}

	resp, err := x.client.Post(x.sessionURL+"/xhr_send", "text/plain", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("resp.Status = %+v\n", resp.Status)
	return nil
}

func (x *XHRSession) Close(status uint32, reason string) error {
	return errors.New("not implemented yet")
}
