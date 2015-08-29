package utils

import (
	"math"
	"math/rand"
	"net"
	"strconv"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// Backoff implements a basic Backoff based on the number of attempts,
// the duration of time that you want to add on each attempt, and the
// maximum backoff duration.
//
// Note that the maxBackoff is a max for the multiplier only. The
// final result can be `maxBackoff / 2 + rand(maxBackoff)` big. In
// other words if the maxBackoff is 60s, then the Backoff returned can
// be anywhere from 30s, to 1m30s.
func Backoff(attempts int, delay, maxBackoff time.Duration) time.Duration {
	fDelay := float64(delay)
	fMaxBackoff := float64(maxBackoff)

	backoff := fDelay * math.Pow(1.6, float64(attempts))

	if backoff > fMaxBackoff {
		backoff = fMaxBackoff
	}

	// Randomize the result
	return time.Duration(backoff/2 + rand.Float64()*backoff)
}

// RandomPort() returns a random port to be used with net.Listen(). It's an
// helper function to register to kontrol before binding to a port. Note that
// this racy, there is a possibility that someoe binds to the port during the
// time you get the port and someone else finds it. Therefore use in caution.
func RandomPort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(port)
}
