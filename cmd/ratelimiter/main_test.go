package main

import (
	"math/rand/v2"
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

func TestGetBucketConcurrentMultipleIPs(t *testing.T) {

	cl := newClientLimiter()

	var wg sync.WaitGroup
	var mu sync.Mutex

	results := make(map[string][]*tokenbucket.TokenBucket)
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"}


	for range 40 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			randomIP := ips[rand.IntN(len(ips))]

			tb := cl.getBucket(randomIP)
			
			mu.Lock()
			defer mu.Unlock()

			results[randomIP] = append(results[randomIP], tb)
		}()
	}

	wg.Wait()

	for ip, buckets := range results {
		first := buckets[0]

		for _, tb := range buckets {
			if tb != first {
				t.Errorf("IP %s: got different bucket pointers for same IP: \nShould be: %p \nInstead got: %p", ip, first, tb)
			}
		}
	}

	type ipBucket struct {
		ip string
		tb *tokenbucket.TokenBucket
	}

	reps := make([]ipBucket, 0)

	for ip, bucket := range results {
		reps = append(reps, ipBucket{ip, bucket[0]})
	}

	for i := 0; i < len(reps); i++ {
		for j := i+1; j < len(reps); j++ {
			if reps[i].tb == reps[j].tb {
            	t.Errorf("IP %s and IP %s got the same bucket pointer", reps[i].ip, reps[j].ip)
			}
		}
	}

}