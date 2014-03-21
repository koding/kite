// +build darwin freebsd linux netbsd openbsd

package kite

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

// SetupSignalHandler listens to SIGUSR1 signal and prints a stackrace for every
// SIGUSR1 signal
func (k *Kite) SetupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for s := range c {
			fmt.Println("Got signal:", s)
			buf := make([]byte, 1<<16)
			runtime.Stack(buf, true)
			fmt.Println(string(buf))
			fmt.Print("Number of goroutines:", runtime.NumGoroutine())
			m := new(runtime.MemStats)
			runtime.GC()
			runtime.ReadMemStats(m)
			fmt.Printf(", Memory allocated: %+v\n", m.Alloc)
		}
	}()
}
