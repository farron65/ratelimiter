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
	"github.com/farron65/ratelimiter/redisslidingwindow"
	"github.com/joho/godotenv"
)

// for general endpoints (token bucket)
const defaultMaxTokens = 10
const defaultRefillRate = 1

// for auth endpoints (token bucket)
const defaultAuthMaxTokens = 5
const defaultAuthRefillRate = 0.01

// for general endpoints (sliding window)
const defaultWindowSize = 60
const defaultLimit = 10

// for auth endpoints (sliding window)
const defaultAuthWindowSize = 60
const defaultAuthLimit = 5

const defaultPort = "8080"
const defaultAlgorithm = "token_bucket"

type Config struct {
	backendURL string
	redisAddr  string
	port       string
	authPaths  map[string]struct{}

	generalLimiter Limiter
	authLimiter    Limiter
}

func loadConfig(slogger *slog.Logger) *Config {

	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		slogger.Warn("invalid backend url", "BACKEND_URL", backendURL)
		log.Fatal("BACKEND_URL is required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		slogger.Warn("invalid Redis address", "REDIS_ADDR", redisAddr)
		log.Fatal("REDIS_ADDR is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		slogger.Warn("invalid env variable", "PORT", os.Getenv("PORT"))
		port = defaultPort
	}

	authPaths := make(map[string]struct{})
	if os.Getenv("AUTH_PATHS") == "" {
		slogger.Warn("AUTH_PATHS not set, using defaults", "AUTH_PATHS", os.Getenv("AUTH_PATHS"))
		authPaths["/login"] = struct{}{}
		authPaths["/signup"] = struct{}{}
	} else {
		for _, path := range strings.Split(os.Getenv("AUTH_PATHS"), ",") {
			authPaths[path] = struct{}{}
		}
	}

	// algorithm-specific vars, only parsed/used for the selected algorithm

	algorithm := os.Getenv("ALGORITHM")
	if algorithm == "" {
		slogger.Warn("ALGORITHM not set, using default", "ALGORITHM", defaultAlgorithm)
		algorithm = defaultAlgorithm
	}

	var generalLimiter, authLimiter Limiter

	switch algorithm {
	case "token_bucket":
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

		generalLimiter = redisbucket.NewRedisBucket("general", redisAddr, maxTokens, refillRate)
		authLimiter = redisbucket.NewRedisBucket("auth", redisAddr, authMaxTokens, authRefillRate)

	case "sliding_window":
		windowSize, err := strconv.ParseInt(os.Getenv("WINDOW_SIZE"), 10, 64)
		if err != nil {
			slogger.Warn("invalid env variable", "WINDOW_SIZE", os.Getenv("WINDOW_SIZE"))
			windowSize = defaultWindowSize
		}

		limit, err := strconv.ParseInt(os.Getenv("LIMIT"), 10, 64)
		if err != nil {
			slogger.Warn("invalid env variable", "LIMIT", os.Getenv("LIMIT"))
			limit = defaultLimit
		}

		authWindowSize, err := strconv.ParseInt(os.Getenv("AUTH_WINDOW_SIZE"), 10, 64)
		if err != nil {
			slogger.Warn("invalid env variable", "AUTH_WINDOW_SIZE", os.Getenv("AUTH_WINDOW_SIZE"))
			authWindowSize = defaultAuthWindowSize
		}

		authLimit, err := strconv.ParseInt(os.Getenv("AUTH_LIMIT"), 10, 64)
		if err != nil {
			slogger.Warn("invalid env variable", "AUTH_LIMIT", os.Getenv("AUTH_LIMIT"))
			authLimit = defaultAuthLimit
		}

		generalLimiter = redisslidingwindow.NewRedisSlidingWindowCounter("general", redisAddr, windowSize, limit)
		authLimiter = redisslidingwindow.NewRedisSlidingWindowCounter("auth", redisAddr, authWindowSize, authLimit)

	default:
		log.Fatalf("unknown ALGORITHM: %q (expected token_bucket or sliding_window)", algorithm)
	}

	return &Config{
		backendURL: backendURL,
		redisAddr:  redisAddr,
		port:       port,
		authPaths:  authPaths,

		generalLimiter: generalLimiter,
		authLimiter:    authLimiter,
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

func isAuthRoute(path string, authPaths map[string]struct{}) bool {

	_, ok := authPaths[path]

	if !ok {
		return false
	}
	return true
}

func checkHandler(generalLimiter Limiter, authLimiter Limiter, slogger *slog.Logger, proxy *httputil.ReverseProxy, authPaths map[string]struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIP, err := getIP(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			slogger.Warn("invalid ip address", "error", err.Error(), "ip", r.RemoteAddr)
			fmt.Fprintln(w, "Error: invalid IP Address")
			return
		}

		var limiter Limiter

		if isAuthRoute(r.URL.Path, authPaths) {
			limiter = authLimiter
		} else {
			limiter = generalLimiter
		}

		b, remainingTokens, err := limiter.Allow(userIP)

		if err != nil {

			slogger.Error("redis error", "error", err, "ip", r.RemoteAddr)
			proxy.ServeHTTP(w, r)

		} else if b {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.Capacity()))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remainingTokens, 'f', 2, 64))

			proxy.ServeHTTP(w, r)
		} else {

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.Capacity()))
			w.Header().Set("Retry-After", strconv.Itoa(limiter.RetryAfterSeconds(remainingTokens)))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remainingTokens, 'f', 2, 64))

			w.WriteHeader(http.StatusTooManyRequests)
			slogger.Info("rate limit exceeded", "ip", r.RemoteAddr)

			fmt.Fprintln(w, "Too many requests, wait for token limiter to fill up")
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func main() {
	fmt.Println("Hi")

	err := godotenv.Load()

	if err != nil {
		log.Println("no .env file found, relying on real environment variables")
	}

	slogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	configVars := loadConfig(slogger)
	target, err := url.Parse(configVars.backendURL)

	if err != nil {
		log.Fatal(err)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
		},
	}

	http.HandleFunc("/health", healthHandler)

	http.HandleFunc("/", checkHandler(configVars.generalLimiter, configVars.authLimiter, slogger, proxy, configVars.authPaths))
	log.Fatal(http.ListenAndServe(":"+configVars.port, nil))
}
