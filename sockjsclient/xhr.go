package sockjsclient

import "time"

func NewXHRSession(opts *DialOptions) (*XHRSession, error) {
	return &XHRSession{
		BaseURL: opts.BaseURL,
		Timeout: opts.Timeout,
	}, nil
}

type XHRSession struct {
	BaseURL string
	Timeout time.Duration
}

func (x *XHRSession) ID() string {
	return ""
}

func (x *XHRSession) Recv() (string, error) {
	return "", nil
}

func (x *XHRSession) Send(frame string) error {
	return nil
}

func (x *XHRSession) Close(status uint32, reason string) error {
	return nil
}
