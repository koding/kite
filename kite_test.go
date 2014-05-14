package kite

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/koding/kite/dnode"
	_ "github.com/koding/kite/testutil"
)

func TestMultiple(t *testing.T) {
	t.Skip("Run it manually")
	testDuration := time.Second * 10

	// number of available mathworker kites to be called
	kiteNumber := 100

	// number of exp kites that will call mathwork kites
	clientNumber := 100

	// ports are starting from 6000 up to 6000 + kiteNumber
	port := 6000

	fmt.Printf("Creating %d mathworker kites\n", kiteNumber)
	for i := 0; i < kiteNumber; i++ {
		m := New("mathworker"+strconv.Itoa(i), "0.1."+strconv.Itoa(i))

		m.HandleFunc("square", Square)
		m.Config.DisableAuthentication = true

		go http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port+i), m)
	}

	// Wait until it's started
	time.Sleep(time.Second * 2)

	fmt.Printf("Creating %d exp clients\n", clientNumber)
	clients := make([]*Client, clientNumber)
	for i := 0; i < clientNumber; i++ {
		c := New("exp"+strconv.Itoa(i), "0.0.1").NewClientString("ws://127.0.0.1:" + strconv.Itoa(port+i))
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

				go func(i int) {
					defer wg.Done()
					start := time.Now()

					time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)))

					result, err := clients[i].TellWithTimeout("square", 4*time.Second, 2)
					if err != nil {
						t.Fatal(err)
					}

					elapsedTime := time.Since(start)

					number := result.MustFloat64()

					fmt.Printf("rpc result: %f elapsedTime %f sec\n", number, elapsedTime.Seconds())
				}(i)
			}
		case <-timeout:
			fmt.Println("test stopped")
			t.SkipNow()
		}

	}

	wg.Wait()
}

// Test 2 way communication between kites.
func TestKite(t *testing.T) {
	// Create a mathworker kite
	mathKite := New("mathworker", "0.0.1")
	mathKite.Config.DisableAuthentication = true
	mathKite.HandleFunc("square", Square)
	mathKite.HandleFunc("squareCB", SquareCB)
	mathKite.HandleFunc("sleep", Sleep)
	go http.ListenAndServe("127.0.0.1:3636", mathKite)

	// Wait until it's started
	time.Sleep(time.Second)

	// Create exp2 kite
	exp2Kite := New("exp2", "0.0.1")
	fooChan := make(chan string)
	exp2Kite.HandleFunc("foo", func(r *Request) (interface{}, error) {
		s := r.Args.One().MustString()
		t.Logf("Message received: %s\n", s)
		fooChan <- s
		return nil, nil
	})

	// exp2 connects to mathworker
	remote := exp2Kite.NewClientString("ws://127.0.0.1:3636")
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
