package main

import (
	"github.com/farron65/ratelimiter/tokenbucket"
	"fmt"
	// "time"
)

func main() {
	fmt.Println("Hi")

	tb := tokenbucket.NewTokenBucket(10, 2)

	fmt.Println(tb)
}