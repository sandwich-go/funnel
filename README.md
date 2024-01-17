# 漏斗

```go
type State struct {
    Ready     bool          // return True if there has enough left quota, else False
    Capacity  int64         // funnel capacity
    LeftQuota int64         // funnel left quota after watering
    Interval  time.Duration // -1 if ret[0] is True, else waiting time until there have enough left quota to watering
    EmptyTime time.Duration // waiting time until the funnel is empty
}
```

```go
var quota int64 = 1         // 需要的数量
state, err := f.Watering(context.Background(), quota)
```

## 内存式漏斗
```go
import (
    "context"
    "github.com/sandwich-go/funnel"
    "time"
)

func main(){
    // capacity 为漏斗的容量
    // operations 接受的操作次数
    // seconds 多少秒内接受，即多少秒内可以接受 operations 的操作次数
    f := funnel.NewMemoryFunnel(capacity, operations, seconds)
    state, err := f.Watering(context.Background(), 1)
}
```

## 分布式漏斗
```go
import (
    "context"
    "github.com/sandwich-go/funnel"
    "github.com/sandwich-go/redisson"
    "time"
)

type funnelScriptBuilder struct{ c redisson.Cmdable }
type funnelScript struct{ s redisson.Scripter }

func (s funnelScriptBuilder) Build(src string) funnel.RedisScript {
    return funnelScript{s: s.c.CreateScript(src)}
}

func (s funnelScript) EvalSha(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
    return s.s.EvalSha(ctx, keys, args...).Slice()
}

func (s funnelScript) Eval(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
    return s.s.Eval(ctx, keys, args...).Slice()
}

func NewFunnel(c redisson.Cmdable, key string, capacity, operations int64, seconds time.Duration) funnel.Funnel {
    return funnel.NewRedisFunnel(funnelScriptBuilder{c}, key, capacity, operations, seconds)
}

func main(){
    var c redisson.Cmdable
    // ... 连接 redis ...

    // capacity 为漏斗的容量
    // operations 接受的操作次数
    // seconds 多少秒内接受，即多少秒内可以接受 operations 的操作次数
    f := NewFunnel(c, "redis_funnel_key", capacity, operations, seconds)
    state, err := f.Watering(context.Background(), 1)
}
```