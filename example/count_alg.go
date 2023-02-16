package main

import (
	"fmt"
	"github.com/go-redis/redis"
	"github.com/vearne/ratelimit"
	"math/rand"
	"sync"
	"time"
)

func consume(r ratelimit.Limiter, group *sync.WaitGroup,
	c * ratelimit.Counter, targetCount int) {
	group.Add(1)
	defer group.Done()
	for {
		ok, err := r.Take()
		if err != nil {
			ok = true
			fmt.Println("error", err)
		}
		if ok {
			value := c.Incre()
			if value >= targetCount{
				break
			}
		} else {
			time.Sleep(time.Duration(rand.Intn(10)+1) * time.Millisecond)
		}
	}
}

func main() {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "xxx", // password set
		DB:       0,      // use default DB
	})

	limiter, err := ratelimit.NewCounterRateLimiter(client,
		"key:count",
		time.Second,
		100,
		2,
	)

	if err != nil {
		fmt.Println("error", err)
		return
	}

	var wg sync.WaitGroup
	total := 500
	counter := ratelimit.NewCounter()
	start := time.Now()
	for i := 0; i < 100; i++ {
		go consume(limiter, &wg, counter, total)
	}
	wg.Wait()
	cost := time.Since(start)
	fmt.Println("cost", time.Since(start), "rate", float64(total)/cost.Seconds())
}
