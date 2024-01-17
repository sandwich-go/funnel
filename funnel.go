package funnel

import (
	"context"
	"time"
)

type State struct {
	Ready     bool          // return True if there has enough left quota, else False
	Capacity  int64         // funnel capacity
	LeftQuota int64         // funnel left quota after watering
	Interval  time.Duration // -1 if ret[0] is True, else waiting time until there have enough left quota to watering
	EmptyTime time.Duration // waiting time until the funnel is empty
}

type Funnel interface {
	Watering(ctx context.Context, quota int64) (State, error)
}
