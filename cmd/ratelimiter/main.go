package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/farron65/ratelimiter/tokenbucket"
)

func checkHandler(tb *tokenbucket.TokenBucket) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tb.Allow() {
			fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path[1:])
		} else {
			// fmt.Fprintf(w, "Too many requests, wait for token bucket to fill up") 
			// w.WriteHeader(http.StatusTooManyRequests) <---- This won't work because header must be sent first

			//
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, "Too many requests, wait for token bucket to fill up")
		}
	}
}

func main() {
	fmt.Println("Hi")

	tb := tokenbucket.NewTokenBucket(10, 1)

	http.HandleFunc("/", checkHandler(tb))
	log.Fatal(http.ListenAndServe(":8080", nil))
}