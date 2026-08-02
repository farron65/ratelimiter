package main

type Limiter interface {
	Allow(ip string) (bool, float64, error)
	RetryAfterSeconds(float64) int
	Capacity() int
}