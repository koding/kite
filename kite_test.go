package kite

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/koding/kite/config"
	"github.com/koding/kite/dnode"
	_ "github.com/koding/kite/testutil"
)

var (
	benchServer *Kite
	benchKite   *Kite
	benchClient *Client
)

func init() {
	benchServer = newXhrKite("mathworker", "0.0.1")
	benchServer.Config.Port = 3630
	benchServer.Config.DisableAuthentication = true
	benchServer.HandleFunc("ping", func(r *Request) (interface{}, error) {
		return "pong", nil
	})
	go benchServer.Run()
	<-benchServer.ServerReadyNotify()

	benchKite = newXhrKite("exp", "0.0.1")
	benchClient = benchKite.NewClient("http://127.0.0.1:3630/kite")
	if err := benchClient.Dial(); err != nil {
		log.Fatal(err)
	}

}

func TestSingle(t *testing.T) {
	for i := 0; i < 10; i++ {
		fmt.Printf("i = %+v\n", i)
		benchClient.Dial()
	}
}

func BenchmarkKiteConnection(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchClient.Dial()
	}
}

func TestMultiple(t *testing.T) {
	testDuration := time.Second * 10

	// number of available mathworker kites to be called
	kiteNumber := 3

	// number of exp kites that will call mathwork kites
	clientNumber := 3

	// ports are starting from 6000 up to 6000 + kiteNumber
	port := 6000

	fmt.Printf("Creating %d mathworker kites\n", kiteNumber)

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

		m.HandleFunc("square", Square)

		go http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port+i), m)
	}

	// Wait until it's started
	time.Sleep(time.Second * 2)

	fmt.Printf("Creating %d exp clients\n", clientNumber)
	clients := make([]*Client, clientNumber)
	for i := 0; i < clientNumber; i++ {
		cn := New("exp"+strconv.Itoa(i), "0.0.1")
		cn.Config.Transport = transport
		c := cn.NewClient("http://127.0.0.1:" + strconv.Itoa(port+i) + "/kite")
		if err := c.Dial(); err != nil {
			t.Fatal(err)
		}

		clients[i] = c
	}

	var wg sync.WaitGroup

	fmt.Printf("Calling mathworker kites with %d conccurent clients randomly\n", clientNumber)
	timeout := time.After(testDuration)

	// every one second
	for {
		select {
		case <-time.Tick(time.Second):
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
		case <-timeout:
			fmt.Println("test stopped")
			t.SkipNow()
		}

	}

	wg.Wait()
}

// Call a single method with multiple clients. This test is implemented to be
// sure the method is calling back with in the same time and not timing out.
func TestConcurrency(t *testing.T) {
	// Create a mathworker kite
	mathKite := newXhrKite("mathworker", "0.0.1")
	mathKite.Config.DisableAuthentication = true
	mathKite.HandleFunc("ping", func(r *Request) (interface{}, error) {
		time.Sleep(time.Second)
		return "pong", nil
	})
	go http.ListenAndServe("127.0.0.1:3637", mathKite)

	// Wait until it's started
	time.Sleep(time.Second)

	// number of exp kites that will call mathworker kite
	clientNumber := 3

	fmt.Printf("Creating %d exp clients\n", clientNumber)
	clients := make([]*Client, clientNumber)
	for i := 0; i < clientNumber; i++ {
		c := newXhrKite("exp", "0.0.1").NewClient("http://127.0.0.1:3637/kite")
		if err := c.Dial(); err != nil {
			t.Fatal(err)
		}

		clients[i] = c
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

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	// Create a mathworker kite
	mathKite := newXhrKite("mathworker", "0.0.1")
	mathKite.Config.Port = 3636
	mathKite.Config.DisableAuthentication = true
	mathKite.HandleFunc("square", Square)
	mathKite.HandleFunc("squareCB", SquareCB)
	mathKite.HandleFunc("sleep", Sleep)

	go mathKite.Run()
	<-mathKite.ServerReadyNotify()

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

	result, err := remote.TellWithTimeout("square", 4*time.Second, 2)
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
