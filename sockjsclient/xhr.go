package sockjsclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

	fmt.Printf("i = %+v\n", i)

	serverID := threeDigits()
	sessionID := randomStringLength(20)
	sessionURL := opts.BaseURL + "/" + serverID + "/" + sessionID

	return &XHRSession{
		client:     client,
		sessionURL: sessionURL,
	}, nil
}

type XHRSession struct {
	client     *http.Client
	sessionURL string
	messages   []string
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
		fmt.Println("sending post")
		resp, err := x.client.Post(x.sessionURL+"/xhr", "text/plain", nil)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		buf := bufio.NewReader(resp.Body)
		frame, err := buf.ReadByte()
		if err != nil {
			return "", err
		}

		fmt.Printf("frame = %+v\n", string(frame))

		switch frame {
		case 'o':
			// session started
			continue
		case 'a':
			var data []byte
			_, err := buf.Read(data)
			if err != nil {
				return "", err
			}

			var messages []string
			err = json.Unmarshal(data, &messages)
			if err != nil {
				return "", err
			}

			x.messages = append(x.messages, messages...)
			break
		case 'h':
			// heartbeat received
			continue
		case 'c':
			// close received
		default:
			return "", errors.New("invalid frame type")
		}
	}

	fmt.Printf("x.messages = %+v\n", x.messages)

	if len(x.messages) == 0 {
		return "", errors.New("no message")
	}
	msg := x.messages[0]
	x.messages = x.messages[1:]
	return msg, nil
}

func (x *XHRSession) Send(frame string) error {
	fmt.Printf("sending frame = %+v\n", frame)
	message := []string{frame}
	body, err := json.Marshal(&message)
	if err != nil {
		return err
	}

	fmt.Printf("string(body) = %+v\n", string(body))

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
