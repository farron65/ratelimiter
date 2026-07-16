package redisbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisBucket struct {
	rd *redis.Client
	maxTokens int
	refillRate float64 
}

type BucketState struct {
	Tokens float64 `json:"tokens"`
	LastRefillTime time.Time `json:"lastRefillTime"`
}

func (state *BucketState) Refill(rb *RedisBucket) {
	elapsedTime := time.Since(state.LastRefillTime)

	tokensToAdd := elapsedTime.Seconds() * rb.refillRate

	state.Tokens = min(float64 (rb.maxTokens), state.Tokens + tokensToAdd)
	state.LastRefillTime = time.Now()
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
	}
}

func (rb *RedisBucket) Allow(ip string) (bool, error) {
	ctx := context.Background()
	key := "ratelimit:" + ip

	var state BucketState

	val, err := rb.rd.Get(ctx, key).Result()

	if err == redis.Nil {
		state = BucketState{
			Tokens: float64(rb.maxTokens),
			LastRefillTime: time.Now(),
		}
	} else if err != nil {
		return false, fmt.Errorf("redis get failed: %w", err)
	} else {
		err := json.Unmarshal([]byte(val), &state)
		if err != nil {
			return false, fmt.Errorf("malformed JSON")
		}
	}

	state.Refill(rb)

	allowed := false
	if state.Tokens >= 1 {
		state.Tokens--
		allowed = true
	}

	serializedState, err := json.Marshal(state)

	if err != nil {
		return false, fmt.Errorf("malformed json")
	}

	er := rb.rd.Set(ctx, key, serializedState, 0).Err()
	if er != nil {
		return false, fmt.Errorf("redis set failed: %w", er)
	}

	return allowed, nil

}