package kontrol

import (
	"sync"
	"time"
)

// OnceEvery is an object that will perform exactly one action every given
// interval.
type OnceEvery struct {
	Interval time.Duration
	ticker   *time.Ticker
	once     sync.Once
	mu       sync.Mutex
	stopped  chan bool
}

// NewOnceEvery creates a new OnceEvery struct
func NewOnceEvery(d time.Duration) *OnceEvery {
	return &OnceEvery{
		Interval: d,
		stopped:  make(chan bool, 1),
	}
}

// Do calls the function f if and only if Do hits the given periodic interval.
// In other words Do can be called multiple times during the interval but it
// get's called only once if it hits the interval tick. So if the interval is
// 10 seconds, and a total of 100 calls are made during this period, once will
// be called it every 10 seconds.
func (o *OnceEvery) Do(f func()) {
	o.mu.Lock()
	if o.ticker == nil {
		o.ticker = time.NewTicker(o.Interval)
	}
	o.mu.Unlock()

	go func() {
		o.once.Do(func() {
			for {
				select {
				case <-o.ticker.C:
					if f == nil {
						continue
					}

					f()
				case <-o.stopped:
					return
				}
			}
		})
	}()
}

// Stop stops the ticker. No other call made with Do will be called anymore.
func (o *OnceEvery) Stop() {
	o.mu.Lock()
	if o.ticker != nil {
		o.ticker.Stop()
	}
	o.mu.Unlock()

	select {
	case <-o.stopped:
	default:
		close(o.stopped)
	}
}
