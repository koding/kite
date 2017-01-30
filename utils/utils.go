package utils

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// globalRand is copied here explicitly to no seed globalRand from math/rand
// and affect its state.
var globalRand = rand.New(&lockedSource{src: rand.NewSource(time.Now().UnixNano() + int64(os.Getpid()))})

type lockedSource struct {
	lk  sync.Mutex
	src rand.Source
}

func (r *lockedSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}

// Int31n returns a pseudo-random int32 in range [0, n).
func Int31n(n int32) int32 {
	return globalRand.Int31n(n)
}

// RandomString returns random string read from crypto/rand.
func RandomString(n int) string {
	p := make([]byte, n/2+1)
	crand.Read(p)
	return hex.EncodeToString(p)[:n]
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
