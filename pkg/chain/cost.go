package chain

import "time"

func TimeCost(now time.Time) int64 {
	return int64(time.Since(now) / time.Millisecond)
}
