package redisbucket_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/farron65/ratelimiter/redisbucket"
)

func TestAllow(t *testing.T) {

	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 10, 1)

	successCount := 0

	for range 50 {
		b, err := rdb.Allow("1.1.1.1")

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

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 10, 1)

	_, err = rdb.Allow("1.1.1.1")

	if err != nil {
		t.Error(err.Error())
	}

	key := "ratelimit:1.1.1.1"

	if !mr.Exists(key) {
		t.Fatal("expected key to exist after Allow()")
	}

	mr.FastForward(20 * time.Second)
	if mr.Exists(key) {
		t.Fatal("expected key to be expired after ttl")
	}

	successCount := 0

	for range 10 {
		b, err := rdb.Allow("1.1.1.1")

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

func TestAllowPartialRefill(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 10, 1)

	fixedTime := time.Now()

	rdb.SetClock(func() time.Time {return fixedTime})

	successCount := 0

	for range 10 {
		_, err := rdb.Allow("1.1.1.1")

		if err != nil {
			t.Error(err.Error())
		}
	}

	b, err := rdb.Allow("1.1.1.1")

	if b {
		t.Errorf("expected to get: %t, instead got: %t", false, b)
	}

	rdb.SetClock(func() time.Time {return fixedTime.Add(5 * time.Second)})

	tokensExpectedToHave := 5

	for range 10 {
		b, err := rdb.Allow("1.1.1.1")

		if err != nil {
			t.Error(err.Error())
		} else if b {
			successCount++
		}
	}

	if successCount != tokensExpectedToHave {
		t.Errorf("expected %d, instead got %d", tokensExpectedToHave, successCount)
	}

}

func TestAllowConcurrencySingleRedisBucket(t *testing.T) {

	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 10, 1)

	var wg sync.WaitGroup

	var successCount atomic.Int64

	for range 25 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			b, err := rdb.Allow("1.1.1.1")

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