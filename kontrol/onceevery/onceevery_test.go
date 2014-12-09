package onceevery

import (
	"testing"
	"time"
)

func TestOnceEvery(t *testing.T) {
	once := New(time.Second)
	count := 0

	go func() {
		for i := 0; i < 100; i++ {
			once.Do(func() {
				count++
			})
		}
	}()

	time.Sleep(time.Second * 2)

	if count != 2 {
		t.Errorf("function should be calle two times, got '%d'", count)
	}
}
