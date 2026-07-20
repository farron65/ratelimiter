package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/farron65/ratelimiter/redisbucket"
)
const defaultMaxTokens = 10
const defaultRefillRate = 1

func getIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		return "", err
	}
	userIP := net.ParseIP(ip)
	if userIP == nil {
		return "", errors.New("invalid IP Address")
	}
	return ip, nil
}

func checkHandler(rb *redisbucket.RedisBucket, slogger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIP, err := getIP(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			slogger.Warn("invalid ip address", "error", err.Error(), "ip", r.RemoteAddr)
			fmt.Fprintln(w, "Error: invalid IP Address",)
			return
		}
		b, err := rb.Allow(userIP) 
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			slogger.Error("redis error", "error", err, "ip", r.RemoteAddr)

			fmt.Fprintln(w, "Internal server error!")
		} else if b {
			fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path[1:])
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
			slogger.Info("rate limit exceeded","ip", r.RemoteAddr)

			fmt.Fprintln(w, "Too many requests, wait for token bucket to fill up")
		}
	}
}

func main() {
	fmt.Println("Hi")

	rb := redisbucket.NewRedisBucket("localhost:6379", defaultMaxTokens, defaultRefillRate)

	slogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	http.HandleFunc("/", checkHandler(rb, slogger))
	log.Fatal(http.ListenAndServe(":8080", nil))
}