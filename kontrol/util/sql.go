package util

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	// PingTimeoutFrequency is the number of times a second to send a
	// ping.
	PingTimeoutFrequency = 5
)

func PingTimeout(db *sql.DB, timeout time.Duration) error {
	milliseconds := (time.Millisecond * 1000) / PingTimeoutFrequency
	retry := ((time.Millisecond * 1000) / milliseconds) * timeout
	for retry > 0 {
		time.Sleep(milliseconds)

		err := db.Ping()
		if err == nil {
			return nil
		}

		retry -= 1
	}
	return fmt.Errorf("Couldn't connect")
}
