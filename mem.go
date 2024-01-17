package funnel

import (
	"context"
	"math"
	"sync"
	"time"
)

type memFunnel struct {
	Capacity    int64   // 漏斗容量
	LeakingRate float64 // 漏水速率

	mx        sync.Mutex
	LeftQuote int64 // 漏斗剩余空间
	LeakingTs int64 // 上一次漏水时间
}

// NewMemoryFunnel 创建内存漏斗
// capacity 为漏斗容量
// operations 为操作数
// seconds 表示多少秒的时间内可进行 operations 次操作
func NewMemoryFunnel(capacity, operations int64, seconds time.Duration) Funnel {
	if seconds <= 0 {
		seconds = 1 * time.Second
	}
	return &memFunnel{
		Capacity:    capacity,
		LeakingRate: float64(operations) / seconds.Seconds(),
		LeftQuote:   capacity,
		LeakingTs:   unix(),
	}
}

func (m *memFunnel) makeSpaceLocked() {
	nowTs := unix()
	deltaTs := nowTs - m.LeakingTs
	deltaQuota := int64(math.Floor(float64(deltaTs) * m.LeakingRate))
	if deltaQuota < 0 {
		// 长时间未操作，溢出
		m.LeftQuote = m.Capacity
		m.LeakingTs = nowTs
	} else if deltaQuota > 0 {
		m.LeftQuote += deltaQuota
		m.LeftQuote = minInt64(m.LeftQuote, m.Capacity)
		m.LeakingTs = nowTs
	}
}

func (m *memFunnel) wateringLocked(quota int64) bool {
	m.makeSpaceLocked()
	if m.LeftQuote >= quota {
		m.LeftQuote -= quota
		return true
	}
	return false
}

func (m *memFunnel) Watering(_ context.Context, quota int64) (State, error) {
	if quota <= 0 {
		quota = 1
	}
	m.mx.Lock()
	ready := m.wateringLocked(quota)
	var capacity, leftQuote, leakingRate = m.Capacity, m.LeftQuote, m.LeakingRate
	m.mx.Unlock()

	var interval float64 = -1
	var emptySec float64 = 0
	if !ready {
		interval = float64(quota) / leakingRate
	}
	if n := capacity - leftQuote; n > 0 {
		emptySec = float64(n) / leakingRate
	}
	var state = State{}
	state.Ready = ready
	state.Capacity = capacity
	state.LeftQuota = leftQuote
	state.Interval = time.Duration(interval * float64(time.Second))
	state.EmptyTime = time.Duration(emptySec * float64(time.Second))
	return state, nil
}
