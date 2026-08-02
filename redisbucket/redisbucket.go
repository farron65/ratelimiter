package redisbucket

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	_ "embed"
)

//go:embed scripts/allow.lua
var allowScript string

var allowScriptObj = redis.NewScript(allowScript)

type RedisBucket struct {
	keyPrefix  string
	rd         *redis.Client
	maxTokens  int
	refillRate float64
	ttl        time.Duration
	now        func() time.Time
}

func NewRedisBucket(keyPrefix string, addr string, maxTokens int, refillRate float64) *RedisBucket {
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

	return &RedisBucket{
		keyPrefix:  keyPrefix,
		rd:         redis.NewClient(opts),
		maxTokens:  maxTokens,
		refillRate: refillRate,
		ttl:        time.Second * time.Duration(math.Ceil(float64(maxTokens)/refillRate)),
		now:        time.Now,
	}
}

func (rb *RedisBucket) SetClock(fn func() time.Time) {
	rb.now = fn
}

func (rb *RedisBucket) RetryAfterSeconds(remaining float64) int {
	return rb.SecondsToNextToken(remaining)
}

func (rb *RedisBucket) Capacity() int {
	return rb.maxTokens
}

func (rb *RedisBucket) Allow(ip string) (bool, float64, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	key := fmt.Sprintf("ratelimit:%s:%s", rb.keyPrefix, ip)

	now := float64(rb.now().UnixNano()) / 1e9
	ttlSeconds := rb.ttl.Seconds()

	result, err := allowScriptObj.Run(ctx, rb.rd, []string{key}, now, rb.maxTokens, rb.refillRate, ttlSeconds).Result()
	if err != nil {
		return false, 0, fmt.Errorf("redis script failed: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	firstValue, ok := resultSlice[0].(int64)

	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	secondValue, ok := resultSlice[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected script result type")
	}

	// scale down by 100 to undo the fixed-point encoding (see allow.lua)
	return firstValue == 1, float64(secondValue) / 100, nil
}

func (rb *RedisBucket) SecondsToNextToken(availableTokens float64) int {
	return int(math.Ceil((1 - availableTokens) / rb.refillRate))
}

func (rb *RedisBucket) MaxTokens() int {
	return rb.maxTokens
}
