package redisbucket

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	_ "embed"
)

//go:embed scripts/allow.lua
var allowScript string

var allowScriptObj = redis.NewScript(allowScript)

type RedisBucket struct {
	rd *redis.Client
	maxTokens int
	refillRate float64 
	ttl time.Duration
}


func NewRedisBucket(addr string, maxTokens int, refillRate float64) *RedisBucket {
	return &RedisBucket{
		rd: redis.NewClient(&redis.Options{
				Addr: addr,
				Password: "",
				DB: 0,
			}),
		maxTokens: maxTokens,
		refillRate: refillRate,
		ttl: time.Second * time.Duration(float64(maxTokens) / refillRate),
	}
}

func (rb *RedisBucket) Allow(ip string) (bool, error) {
	ctx := context.Background()
	key := "ratelimit:" + ip

	now := float64(time.Now().UnixNano()) / 1e9
	ttlSeconds := rb.ttl.Seconds()

	result, err := allowScriptObj.Run(ctx, rb.rd, []string{key}, now, rb.maxTokens, rb.refillRate, ttlSeconds).Result()

	if err != nil {
		return false, fmt.Errorf("redis script failed: %w", err)
	}

	allowed, ok := result.(int64)

	if !ok {
		return false, fmt.Errorf("unexpected script result type")
	}

	return allowed == 1, nil

}