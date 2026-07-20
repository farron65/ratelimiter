package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/farron65/ratelimiter/redisbucket"
	"github.com/joho/godotenv"
)

// for general endpoints
const defaultMaxTokens = 10
const defaultRefillRate = 1

// for auth endpoints
const defaultAuthMaxTokens = 5
const defaultAuthRefillRate = 0.01

type Config struct {
	maxTokens  int
	refillRate float64

	backendURL string

	authMaxTokens  int
	authRefillRate float64
}

func loadConfig(slogger *slog.Logger) *Config {
	maxTokens, err := strconv.Atoi(os.Getenv("MAX_TOKENS"))
	if err != nil {
		slogger.Warn("invalid env variable", "MAX_TOKENS", os.Getenv("MAX_TOKENS"))
		maxTokens = defaultMaxTokens
	}

	refillRate, err := strconv.ParseFloat(os.Getenv("REFILL_RATE"), 64)

	if err != nil {
		slogger.Warn("invalid env variable", "REFILL_RATE", os.Getenv("REFILL_RATE"))
		refillRate = defaultRefillRate
	}

	backendURL := os.Getenv("BACKEND_URL")

	if backendURL == "" {
		slogger.Warn("invalid backend url", "BACKEND_URL", backendURL)
		log.Fatal("BACKEND_URL is required")
	}

	authMaxTokens, err := strconv.Atoi(os.Getenv("AUTH_MAX_TOKENS"))
	if err != nil {
		slogger.Warn("invalid env variable", "AUTH_MAX_TOKENS", os.Getenv("AUTH_MAX_TOKENS"))
		authMaxTokens = defaultAuthMaxTokens
	}

	authRefillRate, err := strconv.ParseFloat(os.Getenv("AUTH_REFILL_RATE"), 64)

	if err != nil {
		slogger.Warn("invalid env variable", "AUTH_REFILL_RATE", os.Getenv("AUTH_REFILL_RATE"))
		authRefillRate = defaultAuthRefillRate
	}

	return &Config{
		maxTokens: maxTokens,
		refillRate: refillRate,

		backendURL: backendURL,

		authMaxTokens: authMaxTokens,
		authRefillRate: authRefillRate,
	}
}

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

func checkHandler(generalBucket *redisbucket.RedisBucket, authBucket *redisbucket.RedisBucket, slogger *slog.Logger, proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIP, err := getIP(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			slogger.Warn("invalid ip address", "error", err.Error(), "ip", r.RemoteAddr)
			fmt.Fprintln(w, "Error: invalid IP Address")
			return
		}

		var b bool

		if strings.HasPrefix(r.URL.Path, "/login") || strings.HasPrefix(r.URL.Path, "/signup") {
			b, err = authBucket.Allow(userIP)
		} else {
			b, err = generalBucket.Allow(userIP)
		}

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			slogger.Error("redis error", "error", err, "ip", r.RemoteAddr)

			fmt.Fprintln(w, "Internal server error!")
		} else if b {
			proxy.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
			slogger.Info("rate limit exceeded", "ip", r.RemoteAddr)

			fmt.Fprintln(w, "Too many requests, wait for token bucket to fill up")
		}
	}
}

func main() {
	fmt.Println("Hi")

	err := godotenv.Load()

	if err != nil {
		log.Println("no .env file found, relying on real environment variables")
	}

	slogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	configVars := loadConfig(slogger)

	generalBucket := redisbucket.NewRedisBucket("general", "localhost:6379", configVars.maxTokens, configVars.refillRate)
	authBucket := redisbucket.NewRedisBucket("auth", "localhost:6379", configVars.authMaxTokens, configVars.authRefillRate)

	target, err := url.Parse(configVars.backendURL)

	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	http.HandleFunc("/", checkHandler(generalBucket, authBucket, slogger, proxy))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
