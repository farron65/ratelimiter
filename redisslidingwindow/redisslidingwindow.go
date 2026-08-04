package redisslidingwindow

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	_ "embed"
)

//go:embed scripts/allow.lua
var allowScript string

var allowScriptObj = redis.NewScript(allowScript)

type RedisSlidingWindowCounter struct {
	keyPrefix string
	rd *redis.Client

	windowSize int64
	limit      int64

	ttl time.Duration
	now func() time.Time
}

func NewRedisSlidingWindowCounter(keyPrefix string, addr string, windowSize int64, limit int64) *RedisSlidingWindowCounter {
	var opts *redis.Options

	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		var err error

		opts, err = redis.ParseURL(addr)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		opts = &redis.Options{Addr: addr}
	}

	opts.MaxRetries = 2 

	return &RedisSlidingWindowCounter{
		keyPrefix: keyPrefix,
		rd: redis.NewClient(opts),

		windowSize: windowSize,
		limit: limit,

		ttl: time.Second * time.Duration(2 * windowSize),
		now:        time.Now,
	}
}

func (sw *RedisSlidingWindowCounter) SetClock(fn func() time.Time) {
	sw.now = fn
}

func (sw *RedisSlidingWindowCounter) RetryAfterSeconds(_ float64) int {
	// remaining unused: sliding window's retry math is time-based, not count-based
	now := sw.now().Unix()
	windowID := now / sw.windowSize
	windowStart := windowID * sw.windowSize
	elapsed := now - windowStart
	secondsLeft := float64(sw.windowSize - elapsed)

	return int(secondsLeft)
}

func (sw *RedisSlidingWindowCounter) Capacity() int {
	return int(sw.limit)
}

func (sw *RedisSlidingWindowCounter) PoolStats() *redis.PoolStats {
    return sw.rd.PoolStats()
}

func (sw *RedisSlidingWindowCounter) Allow(ip string) (bool, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 200 * time.Millisecond)
	defer cancel()

	key := fmt.Sprintf("ratelimit:%s:%s", sw.keyPrefix, ip)

	now := sw.now().Unix()
	ttlSeconds := sw.ttl.Seconds()

	result, err := allowScriptObj.Run(ctx, sw.rd, []string{key}, now, sw.windowSize, sw.limit, ttlSeconds).Result()

	if err != nil {
		return false, 0, fmt.Errorf("redis script failed: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	isAllowed, ok := resultSlice[0].(int64)

	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	remaining, ok := resultSlice[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	return isAllowed == 1, float64(remaining), nil
}