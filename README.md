# Distributed Rate-Limiting Library Using Redis

### Overview
This library is designed to implement distributed rate limiting in a simple, no-nonsense way. Similar to how an ID generator works, the client retrieves data from Redis in batches (essentially just values). As long as these values aren't consumed, the rate limit is not exceeded.

### Key Advantages
* Minimal dependencies—only Redis is required, no additional services
* Utilizes Redis' internal clock, so clients don't need synchronized clocks
* Thread- and coroutine-safe
* Low overhead on the system and minimal pressure on Redis

### Important Notes
Different limiter types use different Redis key data structures, so they cannot share the same Redis key name.

Example:
```
127.0.0.1:6379> type key:leaky
string
127.0.0.1:6379> type key:token
hash
127.0.0.1:6379> hgetall key:token
"token_count"
"0"
"updateTime"
"1613805726567122"
127.0.0.1:6379> get key:leaky
"1613807035353864"
```

### Installation
```
go get github.com/arpan491/API-RateLimiter
```

### Usage

#### 1. Create a Redis client
Using the `"github.com/go-redis/redis"` package, the library supports both master-slave and cluster Redis modes.
```
client := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "xxx", // no password
    DB:       0,     // default DB
})
```
Or, for Redis cluster mode:
```
client := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs:    []string{"127.0.0.1:6379"},
    Password: "xxxx",
})
```

#### 2. Create a RateLimiter

For a token bucket rate limiter that allows 200 operations per second:
```
limiter, err := ratelimit.NewTokenBucketRateLimiter(ctx, client, "push", time.Second, 200, 20, 5)
```
For 200 operations per minute:
```
limiter, err := ratelimit.NewTokenBucketRateLimiter(ctx, client, "push", time.Minute, 200, 20, 5)
```

#### 2.1 Counter Algorithm
```
func NewCounterRateLimiter(ctx context.Context, client redis.Cmdable, key string, duration time.Duration,
    throughput int, batchSize int) (Limiter, error)
```

| Parameter  | Description                                                                 |
|------------|-----------------------------------------------------------------------------|
| `key`      | Redis key name                                                              |
| `duration` | Time interval for the allowed operation throughput                           |
| `throughput` | Allowed number of operations within the given time interval                 |
| `batchSize` | Number of operations retrieved from Redis in one batch                      |

#### 2.2 Token Bucket Algorithm
```
func NewTokenBucketRateLimiter(ctx context.Context, client redis.Cmdable, key string, duration time.Duration,
    throughput int, maxCapacity int, batchSize int) (Limiter, error)
```

| Parameter    | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| `key`        | Redis key name                                                              |
| `duration`   | Time interval for the allowed operation throughput                          |
| `throughput` | Number of operations allowed within the given time interval                 |
| `maxCapacity`| Maximum tokens that can be stored in the token bucket                        |
| `batchSize`  | Number of operations retrieved from Redis in one batch                      |

#### 2.3 Leaky Bucket Algorithm
```
func NewLeakyBucketLimiter(ctx context.Context, client redis.Cmdable, key string, duration time.Duration,
    throughput int) (Limiter, error)
```

| Parameter    | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| `key`        | Redis key name                                                              |
| `duration`   | Time interval for the allowed operation throughput                          |
| `throughput` | Number of operations allowed within the given time interval                 |

#### 2.4 Sliding Time Window
```
NewSlideTimeWindowLimiter(throughput int, duration time.Duration, windowBuckets int) (Limiter, error)
```

| Parameter       | Description                                                                 |
|-----------------|-----------------------------------------------------------------------------|
| `duration`      | Time interval for the allowed operation throughput                          |
| `throughput`    | Number of operations allowed within the given time interval                 |
| `windowBuckets` | Number of buckets representing a segment of the time window (`duration/windowBuckets`) |

Note: The sliding window limiter operates in-memory and doesn't use Redis, making it unsuitable for distributed rate-limiting scenarios.

### Example
[More examples](https://github.com/arpan491/API-RateLimiter/tree/main/example)

```go
package main

import (
    "context"
    "fmt"
    "github.com/go-redis/redis/v8"
    "github.com/arpan491/API-RateLimiter"
    slog "github.com/vearne/simplelog"
    "sync"
    "time"
)

func consume(r ratelimit.Limiter, group *sync.WaitGroup, c *ratelimit.Counter, targetCount int) {
    defer group.Done()
    var ok bool
    for {
        ok = true
        err := r.Wait(context.Background())
        slog.Debug("r.Wait:%v", err)
        if err != nil {
            ok = false
            slog.Error("error:%v", err)
        }
        if ok {
            value := c.Incr()
            slog.Debug("---value--:%v", value)
            if value >= targetCount {
                break
            }
        }
    }
}

func main() {
    client := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "xxeQl*@nFE", // password
        DB:       0,            // use default DB
    })

    limiter, err := ratelimit.NewTokenBucketRateLimiter(
        context.Background(),
        client,
        "key:token",
        time.Second,
        10,
        5,
        2,
    )

    if err != nil {
        fmt.Println("error", err)
        return
    }

    var wg sync.WaitGroup
    total := 50
    counter := ratelimit.NewCounter()
    start := time.Now()
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go consume(limiter, &wg, counter, total)
    }
    wg.Wait()
    cost := time.Since(start)
    fmt.Println("cost", cost, "rate", float64(total)/cost.Seconds())
}
```

### Dependency
[go-redis/redis](https://github.com/go-redis/redis)