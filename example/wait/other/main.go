package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/arpan491/API-RateLimiter"
	slog "github.com/vearne/simplelog"
)

func main() {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "xxeQl*@nFE", // password set
		DB:       0,            // use default DB
	})

	limiter, err := ratelimit.NewTokenBucketRateLimiter(
		context.Background(),
		client,
		"key:token",
		time.Second,
		10,
		5,
		2)

	if err != nil {
		fmt.Println("error", err)
		return
	}
	start := time.Now()
	total := 100
	for i := 0; i < total; i++ {
		err = limiter.Wait(context.Background())
		slog.Error("err:%v", err)
	}
	cost := time.Since(start)
	fmt.Println("cost", time.Since(start), "rate", float64(total)/cost.Seconds())
}
