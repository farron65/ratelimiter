package tokenbucket

import (
	"fmt"
	"sync"
	"time"
)

type TokenBucket struct {
	tokens float64
	maxTokens int
	refillRate float64
	lastRefillTime time.Time
	mu sync.Mutex
}

func NewTokenBucket(maxTokens int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens: float64(maxTokens),
		maxTokens: maxTokens,
		refillRate: refillRate,
		lastRefillTime: time.Now(),
	}
}

func (tb *TokenBucket) String() string {
	return fmt.Sprintf("tokens: %.2f\nmaxTokens: %d\nrefillRate: %.2f\nlastRefillTime: %s\n\n", tb.tokens, tb.maxTokens, tb.refillRate, tb.lastRefillTime)
}

func (tb *TokenBucket) Refill() {
	elapsedTime := time.Since(tb.lastRefillTime) // get how much time has passed since last refill, eg 2s

	tokensToAdd := elapsedTime.Seconds() * tb.refillRate // how many tokens to add, eg 4 seconds passed, if r is 2/s, we have 8 tokens

	tb.tokens = min(float64 (tb.maxTokens), tb.tokens + tokensToAdd) // make sure to not go over the limit, eg limit is 10, and we already had 4 tokens, so 10 < 12, we stay with 10
	tb.lastRefillTime = time.Now() // update the last refill time
}

func (tb *TokenBucket) Allow() bool {

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.Refill()

	if tb.tokens >= 1 { // if user has enough token, sub 1 token and return true
		tb.tokens--
		return true
	}

	return false // otherwise return false
}