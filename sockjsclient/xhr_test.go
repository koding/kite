package sockjsclient

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"testing"
	"time"
)

type fakeReader struct {
	err error
	p   chan []byte
}

func (fk *fakeReader) Read(p []byte) (int, error) {
	q, ok := <-fk.p
	if !ok {
		if fk.err == nil {
			return 0, io.EOF
		}
		return 0, fk.err
	}
	if len(q) > len(p) {
		panic(io.ErrShortBuffer)
	}
	return copy(p, q), nil
}

func TestFrameReader(t *testing.T) {
	const timeout = 100 * time.Millisecond

	var (
		doWrite = func(fk *fakeReader, p []byte, _ error) {
			fk.p <- p
			close(fk.p)
		}
		doTimeout = func(fk *fakeReader, p []byte, _ error) {
			time.AfterFunc(2*timeout, func() {
				fk.p <- p
				close(fk.p)
			})
		}
		doError = func(fk *fakeReader, _ []byte, err error) {
			fk.err = err
			close(fk.p)
		}
	)

	cases := map[string]struct {
		write func(*fakeReader, []byte, error)
		p     []byte
		err   error
	}{
		"session open":         {doWrite, []byte{'o'}, nil},
		"session open timeout": {doTimeout, []byte{'o'}, ErrPollTimeout},
		"session open error":   {doError, []byte{'o'}, &net.AddrError{}},
		"message":              {doWrite, []byte(`m"hello world"`), nil},
		"message timeout":      {doTimeout, []byte(`m"hello world"`), ErrPollTimeout},
		"message error":        {doError, []byte(`m"hello world"`), &net.AddrError{}},
	}

	for name, cas := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			fk := &fakeReader{
				p: make(chan []byte, 1),
			}

			fr := &frameReader{
				r:       bufio.NewReader(fk),
				timeout: timeout,
			}

			cas.write(fk, cas.p, cas.err)

			p, err := ioutil.ReadAll(fr)
			if cas.err != nil {
				if err != cas.err {
					t.Fatalf("got %v, want %v", err, cas.err)
				}

				return
			}

			if bytes.Compare(p, cas.p) != 0 {
				t.Fatalf("got %q, want %q", p, cas.p)
			}
		})
	}
}
