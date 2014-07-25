package kite

import (
	"errors"
	"testing"
	"time"
)

func TestMethod_Error(t *testing.T) {
	k := New("testkite", "0.0.1")
	k.Config.DisableAuthentication = true
	k.Config.Port = 9999

	var testError = errors.New("an error")
	k.PreHandleFunc(func(r *Request) (interface{}, error) { return nil, testError })

	// the following shouldn't do anything because the previous error breaks the chain
	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		return "handle", nil
	})
	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post1", nil })
	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post2", nil })

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:9999/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	_, err := c.TellWithTimeout("foo", 4*time.Second)
	if err == nil {
		t.Fatal("PreHandle returns an error, however error is non-nil.")
	}

	if err.Error() != testError.Error() {
		t.Errorf("Error should be '%v', got '%v'", testError, err)
	}
}

func TestMethod_Base(t *testing.T) {
	k := New("testkite", "0.0.1")
	k.Config.DisableAuthentication = true
	k.Config.Port = 9999

	k.PreHandleFunc(func(r *Request) (interface{}, error) {
		return "pre1", nil
	})

	k.PreHandleFunc(func(r *Request) (interface{}, error) {
		if r.Response.(string) != "pre1" {
			t.Errorf("Response from previous pre handler should be pre1, got: %v", r.Response)
		}

		return "pre2", nil
	})

	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		if r.Response.(string) != "funcPre1" {
			t.Errorf("Response from previous pre handler should be funcPre1, got: %v", r.Response)
		}
		return "handle", nil
	}).PreHandleFunc(func(r *Request) (interface{}, error) {
		if r.Response.(string) != "pre2" {
			t.Errorf("Response from previous pre handler should be pre2, got: %v", r.Response)
		}

		return "funcPre1", nil
	}).PostHandleFunc(func(r *Request) (interface{}, error) {
		if r.Response.(string) != "handle" {
			t.Errorf("Response from previous pre handler should be handle, got: %v", r.Response)
		}

		return "funcPost1", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) {
		if r.Response.(string) != "funcPost1" {
			t.Errorf("Response from previous pre handler should be funcPost1, got: %v", r.Response)
		}

		return "post1", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) {
		if r.Response.(string) != "post1" {
			t.Errorf("Response from previous pre handler should be post1, got: %v", r.Response)
		}

		return "post2", nil
	})

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:9999/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	result, err := c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if result.MustString() != "post2" {
		t.Errorf("Latest repsonse should be post2, got %s", result.MustString())
	}

}
