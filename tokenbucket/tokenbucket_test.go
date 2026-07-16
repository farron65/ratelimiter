package tokenbucket_test

import (
	"sync"
	"testing"

	"github.com/farron65/ratelimiter/tokenbucket"
)

func TestAllowConcurrencySingleBucket(t *testing.T) {
	tb := tokenbucket.NewTokenBucket(10, 0)
	
	var wg sync.WaitGroup
	var mu sync.Mutex

	successCount := 0

	for range 100 {
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