package main

import (
	"flag"
	"fmt"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"sync"
	"time"
)

var port = flag.String("port", "", "port to bind itself")
var work sync.WaitGroup

type Exp struct {
}

func main() {
	flag.Parse()
	methods := map[string]interface{}{}
	o := protocol.Options{
		Username: "huseyin",
		Kitename: "application",
		Port:     *port,
	}
	k := kite.New(&o, new(Exp), methods)
	go k.Start()

	// this is needed that the goroutine k.Start() is been settled. We will
	// probably change the behaviour of k.Start() from blocking to nonblocking
	// and remove the sleep, however this is a design decision that needs to be
	// rethought.
	time.Sleep(1 * time.Second)

	squareOf := func(i float64) {
		k.Call("mathworker", "Square", i, func(err error, res string) {
			if err != nil {
				fmt.Println("got an error", err)
			} else {
				fmt.Printf("bitti %s\n", res)
			}

			// notify that our work is done
			work.Done()
		})
	}

	// run ten times asynchronously the Square method, each with a different
	// argument
	// (pro-tip: start multiple mathworkers to distribute the work between them)
	for i := 0; i <= 10; i++ {
		work.Add(1)
		go squareOf(float64(i))
	}

	// wait now until all of our work is done :)
	work.Wait()
	fmt.Println("finished all square methods")
}
