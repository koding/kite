package onceevery

import (
	"sync"
	"time"
)

// OnceEvery is an object that will perform exactly one action every given
// interval.
type OnceEvery struct {
	Interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

// NewOnceEvery creates a new OnceEvery struct
func New(d time.Duration) *OnceEvery {
	return &OnceEvery{
		Interval: d,
	}
}

// Do calls the function f if and only if Do hits the given periodic interval.
// In other words Do can be called multiple times during the interval but it
// gets called only once if it hits the interval tick. So if the interval is
// 10 seconds, and a total of 100 calls are made during this period, f will
// be called it every 10 seconds.
func (o *OnceEvery) Do(f func()) {
	if f == nil {
		panic("passed function is nil")
	}

	o.mu.Lock()
	now := time.Now()
	ok := o.last.Add(o.Interval).Before(now)
	if ok {
		o.last = now
	}
	o.mu.Unlock()

	if ok {
		f()
	}
}
