package funnel

import "time"

var nowFunc = time.Now

func unix() int64 { return nowFunc().Unix() }

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
