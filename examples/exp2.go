package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"math/rand"
	"time"
)

var port = flag.String("port", "", "port to bind itself")

type Exp2 struct {
}

func main() {
	flag.Parse()
	options := &protocol.Options{
		Kitename: "application",
		Version:  "1",
		Port:     *port,
	}

	k := kite.New(options)
	go k.Start()

	// this is needed that the goroutine k.Start() is been settled. We will
	// probably change the behaviour of k.Start() from blocking to nonblocking
	// and remove the sleep, however this is a design decision that needs to be
	// rethought.
	time.Sleep(1 * time.Second)

	squareOf := func(i float64) {
		k.Call("devrim", "mathworker", "Square", i, func(err error, res string) {
			if err != nil {
				fmt.Println("call error:", err)
			} else {
				fmt.Printf("input: %2.0f  rpc result: %s\n", i, res)
			}
		})
	}

	ticker := time.NewTicker(time.Second * 2)

	for {
		select {
		case c := <-ticker.C:
			n := c.Second()
			if n <= 0 {
				n = 60
			}

			go squareOf(float64(rand.Intn(n)))
		}
	}

	fmt.Println("finished all square methods")
}
