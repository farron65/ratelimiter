package main

import (
	"sync"
	"testing"

	"github.com/farron65/ratelimiter/tokenbucket"
)

func TestGetBucketConcurrent(t *testing.T) {
	cl := newClientLimiter()
	
	var wg sync.WaitGroup
	var mu sync.Mutex

	results := make([]*tokenbucket.TokenBucket, 0)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			tb := cl.getBucket("1.1.1.1")
			mu.Lock()
			
			results = append(results, tb)
			defer mu.Unlock()

		}()
	}

	wg.Wait()

	first := results[0]
	for _, tb := range results {
		if tb != first {
			t.Errorf("Expected same pointer for same IP, got different token buckets")
		}
	}
}

func TestAllowConcurrencySingleBucket(t *testing.T) {
	tb := tokenbucket.NewTokenBucket(10, 0)
	
	var wg sync.WaitGroup
	var mu sync.Mutex

	successCount := 0

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			b := tb.Allow()

			mu.Lock()
			defer mu.Unlock()
			if b {successCount ++}
			
		}()
	}

	wg.Wait()

	if successCount != 10 {
		t.Errorf("Expected %d, instead got %d", 10, successCount)
	}
}