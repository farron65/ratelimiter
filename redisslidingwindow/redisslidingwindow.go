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


func (rswc *RedisSlidingWindowCounter) SetClock(fn func() time.Time) {
	rswc.now = fn
}

func (rswc *RedisSlidingWindowCounter) Allow(ip string) (bool, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 200 * time.Millisecond)
	defer cancel()

	key := fmt.Sprintf("ratelimit:%s:%s", rswc.keyPrefix, ip)

	now := rswc.now().Unix()
	ttlSeconds := rswc.ttl.Seconds()

	result, err := allowScriptObj.Run(ctx, rswc.rd, []string{key}, now, rswc.windowSize, rswc.limit, ttlSeconds).Result()

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

	return isAllowed == 1, remaining, nil

}