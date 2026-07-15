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
