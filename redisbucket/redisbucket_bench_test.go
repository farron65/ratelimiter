package redisbucket_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/farron65/ratelimiter/redisbucket"
)

func BenchmarkAllow(b *testing.B) {

	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redisbucket.NewRedisBucket("bench", mr.Addr(), 1000000, 1000000)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		_, _, err := rdb.Allow("1.1.1.1")

		if err != nil {
			b.Fatalf("got an error: %s", err.Error())
		} 
	}
}

func BenchmarkAllowRealRedis(b *testing.B) {

	rdb := redisbucket.NewRedisBucket("bench", "localhost:6379", 1000000, 1000000)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		_, _, err := rdb.Allow("1.1.1.1")

		if err != nil {
			b.Fatalf("got an error: %s", err.Error())
		} 
	}
}