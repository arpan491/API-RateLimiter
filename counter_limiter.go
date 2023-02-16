package ratelimit

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	slog "github.com/vearne/simplelog"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
	"time"
)

//nolint:govet
type CounterLimiter struct {
	BaseRateLimiter
	duration   time.Duration
	throughput int
	batchSize  int
	N          int64
	g          singleflight.Group

	/*
		If the traffic is too large, the limiter will request Redis frequently.
		To avoid this situation, the frequency of accessing Redis will be limited.
	*/
	AntiDDoS        bool
	antiDDoSLimiter *rate.Limiter
}

func NewCounterRateLimiter(ctx context.Context, client redis.Cmdable, key string, duration time.Duration,
	throughput int,
	batchSize int) (Limiter, error) {

	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	if duration < time.Millisecond {
		return nil, errors.New("duration is too small")
	}

	if throughput <= 0 {
		return nil, errors.New("throughput must greater than 0")
	}

	if batchSize <= 0 {
		return nil, errors.New("batchSize must greater than 0")
	}

	script := algMap[CounterAlg]
	scriptSHA1 := fmt.Sprintf("%x", sha1.Sum([]byte(script)))

	r := CounterLimiter{
		BaseRateLimiter: BaseRateLimiter{redisClient: client, scriptSHA1: scriptSHA1, key: key},
		duration:        duration,
		throughput:      throughput,
		batchSize:       batchSize,
		N:               0,
		AntiDDoS:        true,
	}
	r.interval = duration / time.Duration(throughput)

	if !r.redisClient.ScriptExists(ctx, r.scriptSHA1).Val()[0] {
		r.redisClient.ScriptLoad(ctx, script).Val()
	}

	// 2x throughput
	throughputPerSec := int(float64(throughput) / float64(duration/time.Second))
	r.antiDDoSLimiter = rate.NewLimiter(rate.Limit(throughputPerSec*2), throughputPerSec*2)

	return &r, nil
}

// just for test
func (r *CounterLimiter) WithAntiDDos(antiDDoS bool) {
	r.AntiDDoS = antiDDoS
}

func (r *CounterLimiter) tryTakeFromLocal() bool {
	r.Lock()
	defer r.Unlock()
	if r.N > 0 {
		r.N = r.N - 1
		return true
	}
	return false
}

// wait until take a token or timeout
func (r *CounterLimiter) Wait(ctx context.Context) (err error) {
	ok, err := r.Take(ctx)
	slog.Debug("r.Take")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	deadline, ok := ctx.Deadline()
	minWaitTime := r.interval

	slog.Debug("minWaitTime:%v", minWaitTime)
	if ok {
		if deadline.Before(time.Now().Add(minWaitTime)) {
			slog.Debug("can't get token before %v", deadline)
			return fmt.Errorf("can't get token before %v", deadline)
		}
	}

	for {
		timer := time.NewTimer(minWaitTime)
		select {
		// 执行的代码
		case <-ctx.Done():
			return errors.New("context timeout")
		case <-timer.C:
			ok, err := r.Take(ctx)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}

func (r *CounterLimiter) Take(ctx context.Context) (bool, error) {
	// 0. Anti DDoS
	if r.AntiDDoS {
		if !r.antiDDoSLimiter.Allow() {
			return false, nil
		}
	}

	// 1. try to get from local
	if r.tryTakeFromLocal() {
		return true, nil
	}

	// 2. try to get from redis
	_, err, _ := r.g.Do(r.key, func() (interface{}, error) {
		x, err := r.redisClient.EvalSha(
			ctx,
			r.scriptSHA1,
			[]string{r.key},
			int(r.duration/time.Microsecond),
			r.throughput,
			r.batchSize,
		).Result()
		if err != nil {
			return 0, err
		}
		r.Lock()
		r.N = x.(int64)
		r.Unlock()
		return r.N, nil
	})
	if err != nil {
		return false, err
	}

	return r.tryTakeFromLocal(), nil
}
