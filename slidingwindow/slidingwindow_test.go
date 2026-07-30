package slidingwindow_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/farron65/ratelimiter/slidingwindow"
)

func TestAllowConcurrencySingleSlidingWindow(t *testing.T) {
	swc := slidingwindow.NewSlidingWindowCounter(60, 10)

	var wg sync.WaitGroup

	var successCount atomic.Int64

	for range 100 {
		wg.Add(1)

		go func ()  {
			defer wg.Done()
			b := swc.Allow()

			if b {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 10 {
		t.Errorf("Expected %d, instead got %d", 10, successCount.Load())
	}
}

func TestAllowRolloverResetsWindow(t *testing.T) {
	swc := slidingwindow.NewSlidingWindowCounter(60, 10)

	fakeNow := time.Unix(0, 0)

	swc.SetClock(func() time.Time {return fakeNow})

	// Fill up the current window to the limit
    for i := 0; i < 10; i++ {
        if !swc.Allow() {
            t.Fatalf("expected request %d to be allowed", i)
        }
    }

    // 11th request in same window should be denied
	if swc.Allow() {
        t.Fatalf("expected 11th request to be denied")
    }

	// Advance clock to a new window (60s later)
	fakeNow = time.Unix(60, 0)
	swc.SetClock(func() time.Time {return fakeNow})

	// At elapsed=0 in new window, weight=1.0, so full old count (10) carries over
    // estimatedCount = 1.0*10 + 0 = 10, which is NOT < limit(10) -> should still be denied

	if swc.Allow() {
		t.Error("expected request to be denied right at window boundary")
	}

	fakeNow = time.Unix(90, 0)
	swc.SetClock(func() time.Time {return fakeNow})

	if !swc.Allow() {
		t.Error("expected request to be allowed")
	}
}