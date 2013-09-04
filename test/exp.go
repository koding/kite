package main

import (
	"fmt"
	"koding/newkite/kite"
	"sync"
	"time"
)

var work sync.WaitGroup

func main() {
	k := kite.New("application", nil)
	go k.Start()

	// this is needed that the goroutine k.Start() is been settled. We will
	// probably change the behaviour of k.Start() from blocking to nonblocking
	// and remove the sleep, however this is a design decision that needs to be
	// rethought.
	time.Sleep(1 * time.Second)

	squareOf := func(i int) {
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
		go squareOf(i)
	}

	// wait now until all of our work is done :)
	work.Wait()
	fmt.Println("finished all square methods")
}
