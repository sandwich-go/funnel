package funnel

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// RedisScript redis 脚本
type RedisScript interface {
	EvalSha(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error)
	Eval(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error)
}

// RedisScriptBuilder redis 脚本工厂，可以通过 src load
type RedisScriptBuilder interface {
	Build(src string) RedisScript
}

const luaFunnel = `
redis.replicate_commands()

local function now()
    local ts = redis.call('TIME')
    return tostring(ts[1] + ts[2] / 1000000)
end

local Funnel = {}

function Funnel:new(o, capacity, operations, seconds, left_quota, leaking_ts)
    o = o or {}
    setmetatable(o, self)
    self.__index = self
    self.capacity = capacity
    self.operations = operations
    self.seconds = seconds
    self.left_quota = left_quota
    self.leaking_ts = leaking_ts
    self.leaking_rate = operations / seconds
    return o
end

function Funnel:make_space(quota)
    local now_ts = now()
    local delta_ts = now_ts - self.leaking_ts
    local delta_quota = delta_ts * self.leaking_rate
    if (self.left_quota + delta_quota) < quota then
        return
    else
        self.left_quota = self.left_quota + delta_quota
        if self.left_quota > self.capacity then
            self.left_quota = self.capacity
        end
        self.leaking_ts = now_ts
    end
end

function Funnel:watering(quota)
    self:make_space(quota)
    if self.left_quota >= quota then
        self.left_quota = self.left_quota - quota
        return
            0,
            self.capacity,
            self.left_quota,
            tostring(-1.0),
            tostring((self.capacity - self.left_quota) / self.leaking_rate)
    else
        return
            1,
            self.capacity,
            self.left_quota,
            tostring(quota / self.leaking_rate),
            tostring((self.capacity - self.left_quota) / self.leaking_rate)
    end
end

local key =  KEYS[1]
local capacity = tonumber(ARGV[1])
local operations = tonumber(ARGV[2])
local seconds = tonumber(ARGV[3])
local quota = tonumber(ARGV[4])

local left_quota
local leaking_ts
local cache = redis.call('HMGET', key, 'left_quota', 'leaking_ts')
if cache[1] ~= false then
    left_quota = tonumber(cache[1])
    if left_quota > capacity then
        left_quota = capacity
    end
    leaking_ts = cache[2]
else
    left_quota = capacity
    leaking_ts = now()
end

local funnel = Funnel:new(nil, capacity, operations, seconds, left_quota, leaking_ts)
local ready, capacity, left_quota, interval, empty_time = funnel:watering(quota)

redis.call('HMSET', key,
    'left_quota', funnel.left_quota,
    'leaking_ts', funnel.leaking_ts,
    'capacity', funnel.capacity,
    'operations', funnel.operations,
    'seconds', funnel.seconds
)

return {ready, capacity, left_quota, interval, empty_time}`

type redisFunnel struct {
	keys                 []string
	key                  string
	capacity, operations int64
	seconds              float64
	s                    RedisScript
}

// NewRedisFunnel 创建 redis 漏斗
// RedisScriptBuilder 为 redis 脚本工厂
// key 为 redis 中 漏斗的 key 值
// capacity 为漏斗容量
// operations 为操作数
// seconds 表示多少秒的时间内可进行 operations 次操作
func NewRedisFunnel(c RedisScriptBuilder, key string, capacity, operations int64, seconds time.Duration) Funnel {
	if seconds <= 0 {
		seconds = 1 * time.Second
	}
	f := redisFunnel{key: key, capacity: capacity, operations: operations, seconds: seconds.Seconds()}
	f.keys = []string{f.key}
	f.s = c.Build(luaFunnel)
	return f
}

func (r redisFunnel) runScript(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
	ret, err := r.s.EvalSha(ctx, keys, args...)
	if err != nil && strings.HasPrefix(err.Error(), "NOSCRIPT ") {
		ret, err = r.s.Eval(ctx, keys, args...)
	}
	return ret, err
}

func (r redisFunnel) Watering(ctx context.Context, quota int64) (State, error) {
	var state = State{}
	res, err := r.runScript(ctx, r.keys, r.capacity, r.operations, r.seconds, quota)
	if err != nil {
		return state, err
	}
	state.Ready = res[0].(int64) == 0
	state.Capacity = res[1].(int64)
	state.LeftQuota = res[2].(int64)
	interval, _ := strconv.ParseFloat(res[3].(string), 64)
	emptyTime, _ := strconv.ParseFloat(res[4].(string), 64)
	state.Interval = time.Duration(interval * float64(time.Second))
	state.EmptyTime = time.Duration(emptyTime * float64(time.Second))
	return state, err
}
