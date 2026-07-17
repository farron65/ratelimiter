package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"github.com/farron65/ratelimiter/redisbucket"
)
const defaultMaxTokens = 10
const defaultRefillRate = 1

func getIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		return "", errors.New("invalid IP Address")
	}
	userIP := net.ParseIP(ip)
	if userIP == nil {
		return "", errors.New("invalid IP Address")
	}
	return ip, nil
}

func checkHandler(rb *redisbucket.RedisBucket) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIP, err := getIP(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Error: %s", err.Error())
			return
		}
		b, er := rb.Allow(userIP) 
		if er != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error %s!", er.Error())
		} else if b {
			fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path[1:])
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, "Too many requests, wait for token bucket to fill up")
		}
	}
}

func main() {
	fmt.Println("Hi")

	rb := redisbucket.NewRedisBucket("localhost:6379", defaultMaxTokens, defaultRefillRate)

	http.HandleFunc("/", checkHandler(rb))
	log.Fatal(http.ListenAndServe(":8080", nil))
}