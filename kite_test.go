package kite

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	"github.com/koding/kite/protocol"
	"github.com/koding/kite/sockjsclient"
	_ "github.com/koding/kite/testutil"

	"github.com/igm/sockjs-go/sockjs"
)

func init() {
	rand.Seed(time.Now().Unix() + int64(os.Getpid()))
}

func panicHandler(*Client) {
	panic("this panic should be ignored")
}

func panicRegisterHandler(*protocol.RegisterResult) {
	panic("this panic should be ignored")
}

func TestMultiple(t *testing.T) {
	testDuration := time.Second * 10

	// number of available mathworker kites to be called
	kiteNumber := 3

	// number of exp kites that will call mathwork kites
	clientNumber := 3

	// ports are starting from 6000 up to 6000 + kiteNumber
	port := 6000

	var transport config.Transport
	if transportName := os.Getenv("KITE_TRANSPORT"); transportName != "" {
		tr, ok := config.Transports[transportName]
		if !ok {
			t.Fatalf("transport '%s' doesn't exists", transportName)
		}

		transport = tr
	}

	for i := 0; i < kiteNumber; i++ {
		m := New("mathworker"+strconv.Itoa(i), "0.1."+strconv.Itoa(i))
		m.Config.DisableAuthentication = true
		m.Config.Transport = transport
		m.Config.Port = port + i

		m.OnConnect(panicHandler)
		m.OnRegister(panicRegisterHandler)
		m.OnDisconnect(panicHandler)
		m.OnFirstRequest(panicHandler)

		m.HandleFunc("square", Square)
		go m.Run()
		<-m.ServerReadyNotify()
		defer m.Close()
	}

	clients := make([]*Client, clientNumber)
	for i := 0; i < clientNumber; i++ {
		cn := New("exp"+strconv.Itoa(i), "0.0.1")
		cn.Config.Transport = transport

		c := cn.NewClient("http://127.0.0.1:" + strconv.Itoa(port+i) + "/kite")
		if err := c.Dial(); err != nil {
			t.Fatal(err)
		}

		clients[i] = c
		defer c.Close()
	}

	timeout := time.After(testDuration)

	// every one second
	for {
		select {
		case <-time.Tick(time.Second):
			var wg sync.WaitGroup

			for i := 0; i < clientNumber; i++ {
				wg.Add(1)

				go func(i int, t *testing.T) {
					defer wg.Done()
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)))

					_, err := clients[i].TellWithTimeout("square", 4*time.Second, 2)
					if err != nil {
						t.Error(err)
					}
				}(i, t)
			}

			wg.Wait()
		case <-timeout:
			return
		}
	}
}

func TestSendError(t *testing.T) {
	const timeout = 5 * time.Second

	ksrv := newXhrKite("echo-server", "0.0.1")
	ksrv.Config.DisableAuthentication = true
	ksrv.HandleFunc("echo", func(r *Request) (interface{}, error) {
		return r.Args.One().MustString(), nil
	})

	go ksrv.Run()
	<-ksrv.ServerReadyNotify()
	defer ksrv.Close()

	clientSession := make(chan sockjs.Session, 1)

	kcli := newXhrKite("echo-client", "0.0.1")
	kcli.Config.DisableAuthentication = true
	c := kcli.NewClient(fmt.Sprintf("http://127.0.0.1:%d/kite", ksrv.Port()))
	c.testHookSetSession = func(s sockjs.Session) {
		if _, ok := s.(*sockjsclient.XHRSession); ok {
			clientSession <- s
		}
	}

	if err := c.DialTimeout(timeout); err != nil {
		t.Fatalf("DialTimeout()=%s", err)
	}

	select {
	case <-time.After(timeout):
		t.Fatal("timed out waiting for session")
	case c := <-clientSession:
		c.Close(500, "transport closed")
	}

	done := make(chan error)

	go func() {
		_, err := c.Tell("echo", "should fail")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected err != nil, was nil")
		}
	case <-time.After(timeout):
		t.Fatal("timed out waiting for send failure")
	}
}

// Call a single method with multiple clients. This test is implemented to be
// sure the method is calling back with in the same time and not timing out.
func TestConcurrency(t *testing.T) {
	// Create a mathworker kite
	mathKite := newXhrKite("mathworker", "0.0.1")
	mathKite.Config.DisableAuthentication = true
	mathKite.Config.Port = 3637
	mathKite.HandleFunc("ping", func(r *Request) (interface{}, error) {
		time.Sleep(time.Second)
		return "pong", nil
	})
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()
	defer mathKite.Close()

	// number of exp kites that will call mathworker kite
	clientNumber := 3

	clients := make([]*Client, clientNumber)
	for i := range clients {
		c := newXhrKite("exp", "0.0.1").NewClient("http://127.0.0.1:3637/kite")
		if err := c.Dial(); err != nil {
			t.Fatal(err)
		}

		clients[i] = c
		defer c.Close()
	}

	var wg sync.WaitGroup

	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := clients[i].TellWithTimeout("ping", 4*time.Second)
			if err != nil {
				t.Fatal(err)
			}

			if result.MustString() != "pong" {
				t.Errorf("Got %s want: pong", result.MustString())
			}
		}(i)
	}

	wg.Wait()
}

func TestNoConcurrentCallbacks(t *testing.T) {
	const timeout = 2 * time.Second

	type Callback struct {
		Index int
		Func  dnode.Function
	}

	k := newXhrKite("callback", "0.0.1")
	k.Config.DisableAuthentication = true
	k.HandleFunc("call", func(r *Request) (interface{}, error) {
		if r.Args == nil {
			return nil, errors.New("empty argument")
		}

		var arg Callback
		if err := r.Args.One().Unmarshal(&arg); err != nil {
			return nil, err
		}

		if !arg.Func.IsValid() {
			return nil, errors.New("invalid argument")
		}

		if err := arg.Func.Call(arg.Index); err != nil {
			return nil, err
		}

		return true, nil
	})

	go k.Run()
	<-k.ServerReadyNotify()
	defer k.Close()

	url := fmt.Sprintf("http://127.0.0.1:%d/kite", k.Port())

	c := k.NewClient(url)
	defer c.Close()

	// The TestNoConcurrentCallbacks asserts ConcurrentCallbacks
	// are disabled by default for each new client.
	//
	// When callbacks are executed concurrently, the order
	// of indices received on the channel is random,
	// thus making this test to fail.
	//
	// c.ConcurrentCallbacks = true

	if err := c.DialTimeout(timeout); err != nil {
		t.Errorf("DialTimeout(%q)=%s", url, err)
	}

	indices := make(chan int, 50)
	callback := dnode.Callback(func(arg *dnode.Partial) {
		var index int
		if err := arg.One().Unmarshal(&index); err != nil {
			t.Logf("failed to unmarshal: %s", err)
		}

		time.Sleep(time.Duration(rand.Int31n(100)) * time.Millisecond)

		indices <- index
	})

	for i := 0; i < cap(indices); i++ {
		arg := &Callback{
			Index: i + 1,
			Func:  callback,
		}

		if _, err := c.TellWithTimeout("call", timeout, arg); err != nil {
			t.Fatalf("%d: TellWithTimeout()=%s", i, err)
		}
	}

	var n, lastIndex int

	for {
		if n == cap(indices) {
			// All indices were read.
			break
		}

		select {
		case <-time.After(timeout):
			t.Fatalf("reading indices has timed out after %s (n=%d)", timeout, n)
		case index := <-indices:
			if index == 0 {
				t.Fatalf("invalid index=%d (n=%d)", index, n)
			}

			if index <= lastIndex {
				t.Fatalf("expected to receive indices in ascending order; received %d, last index %d (n=%d)", index, lastIndex, n)
			}

			lastIndex = index
			n++
		}
	}
}

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	// Create a mathworker kite
	mathKite := newXhrKite("mathworker", "0.0.1")
	mathKite.Config.DisableAuthentication = true
	mathKite.Config.Port = 3636
	mathKite.HandleFunc("square", Square)
	mathKite.HandleFunc("squareCB", SquareCB)
	mathKite.HandleFunc("sleep", Sleep)
	mathKite.HandleFunc("sqrt", Sqrt)
	mathKite.FinalFunc(func(r *Request, resp interface{}, err error) (interface{}, error) {
		if r.Method != "sqrt" || err != ErrNegative {
			return resp, err
		}

		a := r.Args.One().MustFloat64()

		// JSON does not marshal complex128,
		// for test purpose we use just string
		return fmt.Sprintf("%di", int(math.Sqrt(-a)+0.5)), nil
	})
	go mathKite.Run()
	<-mathKite.ServerReadyNotify()
	defer mathKite.Close()

	// Create exp2 kite
	exp2Kite := newXhrKite("exp2", "0.0.1")
	fooChan := make(chan string)
	exp2Kite.HandleFunc("foo", func(r *Request) (interface{}, error) {
		s := r.Args.One().MustString()
		t.Logf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	})

	// exp2 connects to mathworker
	remote := exp2Kite.NewClient("http://127.0.0.1:3636/kite")
	err := remote.Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()

	result, err := remote.TellWithTimeout("sqrt", 4*time.Second, -4)
	if err != nil {
		t.Fatal(err)
	}

	if s, err := result.String(); err != nil || s != "2i" {
		t.Fatalf("want 2i, got %v (%v)", result, err)
	}

	result, err = remote.TellWithTimeout("square", 4*time.Second, 2)
	if err != nil {
		t.Fatal(err)
	}

	number := result.MustFloat64()

	t.Logf("rpc result: %f\n", number)

	if number != 4 {
		t.Fatalf("Invalid result: %f", number)
	}

	select {
	case s := <-fooChan:
		if s != "bar" {
			t.Fatalf("Invalid message: %s", s)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not get the message")
	}

	resultChan := make(chan float64, 1)
	resultCallback := func(args *dnode.Partial) {
		n := args.One().MustFloat64()
		resultChan <- n
	}

	result, err = remote.TellWithTimeout("squareCB", 4*time.Second, 3, dnode.Callback(resultCallback))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case n := <-resultChan:
		if n != 9.0 {
			t.Fatalf("Unexpected result: %f", n)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Did not get the message")
	}

	result, err = remote.TellWithTimeout("sleep", time.Second)
	if err == nil {
		t.Fatal("Did get message in 1 seconds, however the sleep method takes 2 seconds to response")
	}

	result, err = remote.Tell("sleep")
	if err != nil {
		t.Fatal(err)
	}

	if !result.MustBool() {
		t.Fatal("sleep result must be true")
	}

}

// Sleeps for 2 seconds and returns true
func Sleep(r *Request) (interface{}, error) {
	time.Sleep(time.Second * 2)
	return true, nil
}

// Returns the result. Also tests reverse call.
func Square(r *Request) (interface{}, error) {
	a := r.Args.One().MustFloat64()
	result := a * a

	r.LocalKite.Log.Info("Kite call, sending result '%f' back\n", result)

	// Reverse method call
	r.Client.Go("foo", "bar")

	return result, nil
}

var ErrNegative = errors.New("negative argument")

func Sqrt(r *Request) (interface{}, error) {
	a := r.Args.One().MustFloat64()

	if a < 0 {
		return nil, ErrNegative
	}

	return math.Sqrt(a), nil
}

// Calls the callback with the result. For testing requests with Callback.
func SquareCB(r *Request) (interface{}, error) {
	args := r.Args.MustSliceOfLength(2)
	a := args[0].MustFloat64()
	cb := args[1].MustFunction()

	result := a * a

	r.LocalKite.Log.Info("Kite call, sending result '%f' back\n", result)

	// Send the result.
	err := cb.Call(result)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func newXhrKite(name, version string) *Kite {
	k := New(name, version)
	k.Config.Transport = config.XHRPolling
	return k
}
