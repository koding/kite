package util

import (
	"database/sql"
	"fmt"
	"time"
)

var (
	// PingTimeoutFrequency is the number of times a second to send a
	// ping.
	PingTimeoutFrequency = 5
)

func PingTimeout(db *sql.DB, timeout time.Duration, frequency ...time.Duration) error {
	tick := time.Second / time.Duration(PingTimeoutFrequency)
	if len(frequency) > 0 {
		tick = time.Second / frequency[0]
	}
	t := time.NewTicker(tick)
	for {
		<-t.C
		err := db.Ping()
		if err == nil {
			return nil
		}

		timeout -= tick
		if timeout <= time.Duration(0) {
			break
		}
	}
	t.Stop()
	return fmt.Errorf("Couldn't connect")
}
