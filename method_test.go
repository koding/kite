package kite

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMethod_Throttling(t *testing.T) {
	k := New("testkite", "0.0.1")
	k.Config.DisableAuthentication = true
	k.Config.Port = 9996

	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		return "handle", nil
	}).Throttle(time.Second*2, 30)

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:9996/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	// First let us exhaust the bucket
	for i := 0; i < 20; i++ {
		_, err := c.TellWithTimeout("foo", 4*time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}

	// now, the request 21 should give a requestLimitError
	_, err := c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		if kErr, ok := err.(*Error); ok {
			if kErr.Type != "requestLimitError" {
				t.Fatal(err)
			}
		}
	}

	// now wait until the bucket is filled again
	time.Sleep(time.Second * 2)

	// this shouldn't give any error at all
	_, err = c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMethod_Latest(t *testing.T) {
	k := New("testkite", "0.0.1")
	k.Config.DisableAuthentication = true
	k.Config.Port = 9997

	k.MethodHandling = ReturnLatest

	k.PreHandleFunc(func(r *Request) (interface{}, error) { return nil, nil })
	k.PreHandleFunc(func(r *Request) (interface{}, error) { return "hello", nil })

	// the following shouldn't do anything because the previous error breaks the chain
	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		return "handle", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post1", nil })
	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post2", nil })

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:9997/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	result, err := c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if result.MustString() != "post2" {
		t.Errorf("Latest response should be post2, got %s", result.MustString())
	}

}

func TestMethod_First(t *testing.T) {
	k := New("testkite", "0.0.1")

	k.Config.DisableAuthentication = true
	k.Config.Port = 9998

	k.MethodHandling = ReturnFirst

	k.PreHandleFunc(func(r *Request) (interface{}, error) { return nil, nil })
	k.PreHandleFunc(func(r *Request) (interface{}, error) { return "hello", nil })

	// the following shouldn't do anything because the previous error breaks the chain
	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		return "handle", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post1", nil })
	k.PostHandleFunc(func(r *Request) (interface{}, error) { return "post2", nil })

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:9998/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	result, err := c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if result.MustString() != "hello" {
		t.Errorf("Latest response should be hello, got %s", result.MustString())
	}

}

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

	if !strings.HasPrefix(err.Error(), testError.Error()) {
		t.Errorf("Error should be '%v', got '%v'", testError, err)
	}
}

func TestMethod_Base(t *testing.T) {
	k := New("testkite", "0.0.1")
	k.Config.DisableAuthentication = true
	k.Config.Port = 10000

	k.PreHandleFunc(func(r *Request) (interface{}, error) {
		r.Context.Set("pre1", "pre1")
		return nil, nil
	})

	k.PreHandleFunc(func(r *Request) (interface{}, error) {
		res, _ := r.Context.Get("pre1")
		if res != "pre1" {
			t.Errorf("Context response from previous pre handler should be pre1, got: %v", res)
		}

		r.Context.Set("pre2", "pre2")
		return nil, nil
	})

	k.HandleFunc("foo", func(r *Request) (interface{}, error) {
		res, _ := r.Context.Get("funcPre1")
		if res != "funcPre1" {
			t.Errorf("Context response from previous pre handler should be funcPre1, got: %v", res)
		}

		r.Context.Set("handle", "handle")
		return "main-response", nil
	}).PreHandleFunc(func(r *Request) (interface{}, error) {
		r.Context.Set("funcPre1", "funcPre1")
		return "funcPre1", nil
	}).PostHandleFunc(func(r *Request) (interface{}, error) {
		res, _ := r.Context.Get("handle")
		if res != "handle" {
			t.Errorf("Context response from previous pre handler should be handle, got: %v", res)
		}

		r.Context.Set("funcPost1", "funcPost1")
		return "funcPost1", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) {
		res, _ := r.Context.Get("funcPost1")
		if res != "funcPost1" {
			t.Errorf("Context response from previous pre handler should be funcPost1, got: %v", res)
		}

		r.Context.Set("post1", "post1")
		return "post1", nil
	})

	k.PostHandleFunc(func(r *Request) (interface{}, error) {
		res, _ := r.Context.Get("post1")
		if res != "post1" {
			t.Errorf("Context response from previous pre handler should be post1, got: %v", res)
		}

		return "post2", nil
	})

	go k.Run()
	defer k.Close()
	<-k.ServerReadyNotify()

	c := New("exp", "0.0.1").NewClient("http://127.0.0.1:10000/kite")
	if err := c.Dial(); err != nil {
		t.Fatal(err)
	}

	result, err := c.TellWithTimeout("foo", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if result.MustString() != "main-response" {
		t.Errorf("Latest response should be main-response, got %s", result.MustString())
	}

}
