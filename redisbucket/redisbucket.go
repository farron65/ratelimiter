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
	keyPrefix string
	rd *redis.Client
	maxTokens int
	refillRate float64 
	ttl time.Duration
	now func() time.Time
}

func NewRedisBucket(keyPrefix string, addr string, maxTokens int, refillRate float64) *RedisBucket {
	return &RedisBucket{
		keyPrefix: keyPrefix,
		rd: redis.NewClient(&redis.Options{
				Addr: addr,
				Password: "",
				DB: 0,
			}),
		maxTokens: maxTokens,
		refillRate: refillRate,
		ttl: time.Second * time.Duration(float64(maxTokens) / refillRate),
		now: time.Now,
	}
}

func (rdb * RedisBucket) SetClock(fn func() time.Time) {
	rdb.now = fn
}

func (rb *RedisBucket) Allow(ip string) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("ratelimit:%s:%s", rb.keyPrefix, ip)

	now := float64(rb.now().UnixNano()) / 1e9
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