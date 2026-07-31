package redisslidingwindow_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/farron65/ratelimiter/redisslidingwindow"
)

func TestAllow(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rswc := redisslidingwindow.NewRedisSlidingWindowCounter("testing", mr.Addr(), 60, 10)

	successCount := 0

	for range 50 {
		b, _, err := rswc.Allow("1.1.1.1")

		if err != nil {
			t.Errorf("got an error: %s", err.Error())
		} else if b {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("expected to get: %d, instead got: %d", 10, successCount)
	}
}

func TestAllowExpiredKey(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rswc := redisslidingwindow.NewRedisSlidingWindowCounter("testing", mr.Addr(), 60, 10)

	_, _, err = rswc.Allow("1.1.1.1")

	if err != nil {
		t.Fatal(err)
	}

	key := fmt.Sprintf("ratelimit:%s:%s", "testing", "1.1.1.1")

	if !mr.Exists(key) {
		t.Fatal("expected key to EXIST after Allow()")
	}

	mr.FastForward(180 * time.Second)

	if mr.Exists(key) {
		t.Fatal("expected key to be EXPIRED after ttl")
	}

	successCount := 0

	for range 10 {
		b, _, err := rswc.Allow("1.1.1.1")

		if err != nil {
			t.Errorf("got an error: %s", err.Error())
		} else if b {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("expected to get: %d, instead got: %d", 10, successCount)
	}
}

func TestAllowRolloverDecay(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rswc := redisslidingwindow.NewRedisSlidingWindowCounter("testing", mr.Addr(), 60, 10)
	
	fixedTime := time.Unix(0, 0)

	rswc.SetClock(func() time.Time { return fixedTime })

	for  range 10 {
		_, _, err = rswc.Allow("1.1.1.1")
		if err != nil {
			t.Error(err.Error())
		}
	}

	b, _, err := rswc.Allow("1.1.1.1")

	if b {
		t.Errorf("expected to get: %t, instead got: %t", false, b)
	}

	rswc.SetClock(func() time.Time {return fixedTime.Add(90 * time.Second)})

	remainingRequestsToHave := 4

	b, gotRemaining, err := rswc.Allow("1.1.1.1")

	if err != nil {
		t.Error(err.Error())
	}

	if !b {
		t.Errorf("expected to get: %t, instead got: %t", true, b)
	}

	if gotRemaining != int64(remainingRequestsToHave) {
		t.Errorf("expected %d, instead got %d", remainingRequestsToHave, gotRemaining)
	}
}

func TestAllowConcurrencyRedisSingleSlidingWindow(t *testing.T) {

	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rswc := redisslidingwindow.NewRedisSlidingWindowCounter("testing", mr.Addr(), 60, 10)

	var wg sync.WaitGroup
	var successCount atomic.Int64

	for range 25 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			b, _, err := rswc.Allow("1.1.1.1")
			if err != nil {
				t.Error(err.Error())
			}
			if b {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 10 {
		t.Errorf("expected %d, instead got %d", 10, successCount.Load())
	}
}

func TestTTLCeiling(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rswc := redisslidingwindow.NewRedisSlidingWindowCounter("testing", mr.Addr(), 60, 10)

	_, _, err = rswc.Allow("1.1.1.1")

	if err != nil {
		t.Fatal(err)
	}

	mr.FastForward(119 * time.Second)

	key := fmt.Sprintf("ratelimit:%s:%s", "testing", "1.1.1.1")

	if !mr.Exists(key) {
		t.Fatal("expected key to EXIST, but it is expired")
	}

	mr.FastForward(1 * time.Second)

	if mr.Exists(key) {
		t.Fatal("expected key to NOT exist, but it is expired")
	}
}