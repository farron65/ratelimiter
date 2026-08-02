package redisbucket_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/farron65/ratelimiter/redisbucket"
)

func BenchmarkAllow(b *testing.B) {

	mr, _ := miniredis.Run()
	defer mr.Close()

	rb := redisbucket.NewRedisBucket("bench", mr.Addr(), 1000000, 1000000)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		_, _, err := rb.Allow("1.1.1.1")

		if err != nil {
			b.Fatalf("got an error: %s", err.Error())
		} 
	}
}

func BenchmarkAllowRealRedis(b *testing.B) {

	rb := redisbucket.NewRedisBucket("bench", "localhost:6379", 1000000, 1000000)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		_, _, err := rb.Allow("1.1.1.1")

		if err != nil {
			b.Fatalf("got an error: %s", err.Error())
		} 
	}
}

func BenchmarkAllowRealRedisParallel(b *testing.B) {
	rb := redisbucket.NewRedisBucket("bench", "localhost:6379", 1000000, 1000000)

	var counter int64

	before := rb.PoolStats()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddInt64(&counter, 1)
			ip := fmt.Sprintf("10.0.%d.%d", (n/256)%256, n%256)
			_, _, err := rb.Allow(ip)

			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()

	after := rb.PoolStats()

	b.Logf("before: TotalConns=%d Hits=%d Misses=%d Timeouts=%d StaleConns=%d",
		before.TotalConns, before.Hits, before.Misses, before.Timeouts, before.StaleConns)
	b.Logf("after:  TotalConns=%d Hits=%d Misses=%d Timeouts=%d StaleConns=%d",
		after.TotalConns, after.Hits, after.Misses, after.Timeouts, after.StaleConns)
}