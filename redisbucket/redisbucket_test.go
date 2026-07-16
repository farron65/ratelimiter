package redisbucket_test

import (
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

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 10, 0)

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
		t.Errorf("supposed to get: %d, instead got: %d", 10, successCount)
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
		t.Fatal("Expected key to exist after Allow()")
	}

	mr.FastForward(20 * time.Second)
	if mr.Exists(key) {
		t.Fatal("Expected key to be expired after ttl")
	}
}