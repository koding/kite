package onceevery

import (
	"sync"
	"testing"
	"time"
)

func TestOnceEvery(t *testing.T) {
	var wg sync.WaitGroup
	var start time.Time
	count := 0
	interval := 500 * time.Millisecond
	once := New(interval)
	done := make(chan struct{})
	wg.Add(1)

	go func() {
		defer wg.Done()
		start = time.Now()
		for {
			select {
			case <-done:
				return
			default:
				once.Do(func() {
					count++
				})
			}
		}
	}()

	time.Sleep(time.Second * 2)

	close(done)
	wg.Wait()

	n := int(time.Now().Sub(start) / interval)

	// test against range to account runtime scheduling
	if n-1 > count || count > n+1 {
		t.Errorf("want count ∈ [%d, %d]; got %d", n-1, n+1, count)
	}
}
