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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	trustedProxies []*net.IPNet
	clientIPHeader string
}

// Prometheus
var requestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "ratelimiter_requests_total",
		Help: "Requests handled by the rate limiter, by result.",
	},
	[]string{"result"},
)

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

	rawProxies := os.Getenv("TRUSTED_PROXIES")
	var trustedProxies []*net.IPNet
	if rawProxies == "" {
		slogger.Info("TRUSTED_PROXIES not set, ignoring X-Forwarded-For")
	} else {
		for _, s := range strings.Split(rawProxies, ",") {
			_, ipnet, err := net.ParseCIDR(strings.TrimSpace(s))
			if err != nil {
				log.Fatalf("invalid TRUSTED_PROXIES entry %q: %v", s, err)
			}
			trustedProxies = append(trustedProxies, ipnet)

		}
	}

	clientIPHeader := os.Getenv("CLIENT_IP_HEADER")
	return &Config{
		backendURL: backendURL,
		redisAddr:  redisAddr,
		port:       port,
		authPaths:  authPaths,

		generalLimiter: generalLimiter,
		authLimiter:    authLimiter,
		trustedProxies: trustedProxies,
		clientIPHeader: clientIPHeader,
	}
}

func isTrusted(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false

}

func getIP(r *http.Request, nets []*net.IPNet, header string) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		return "", err
	}

	userIP := net.ParseIP(ip)
	if userIP == nil {
		return "", errors.New("invalid IP Address")
	}

	// 1. peer not trusted -> ignore the header
	if !isTrusted(userIP, nets) {
		return ip, nil
	}

	if header != "" {
		if v := net.ParseIP(strings.TrimSpace(r.Header.Get(header))); v != nil {
			return v.String(), nil
		}
	}

	// 2. peer trusted, no header -> nothing to read
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ip, nil
	}

	parts := strings.Split(xff, ",")

	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[i]))
		if candidate == nil {
			return ip, nil
		}
		if !isTrusted(candidate, nets) {
			return candidate.String(), nil
		}
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

func checkHandler(generalLimiter Limiter, authLimiter Limiter, slogger *slog.Logger, proxy *httputil.ReverseProxy, authPaths map[string]struct{}, nets []*net.IPNet, header string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slogger.Info("ip debug",
		"remote_addr", r.RemoteAddr,
		"xff", r.Header.Get("X-Forwarded-For"),
		"true_client_ip", r.Header.Get("True-Client-IP"),
		"cf_connecting_ip", r.Header.Get("CF-Connecting-IP"),
		"x_real_ip", r.Header.Get("X-Real-IP"),
		)
		userIP, err := getIP(r, nets, header)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			slogger.Warn("invalid ip address", "error", err.Error(), "remote_addr", r.RemoteAddr)
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

			slogger.Error("redis error", "error", err, "ip", userIP)
			proxy.ServeHTTP(w, r)

		} else if b {
			requestsTotal.WithLabelValues("allowed").Inc()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.Capacity()))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remainingTokens, 'f', 2, 64))

			proxy.ServeHTTP(w, r)
		} else {
			requestsTotal.WithLabelValues("rejected").Inc()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.Capacity()))
			w.Header().Set("Retry-After", strconv.Itoa(limiter.RetryAfterSeconds(remainingTokens)))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remainingTokens, 'f', 2, 64))

			w.WriteHeader(http.StatusTooManyRequests)
			slogger.Info("rate limit exceeded", "ip", userIP)

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
			r.SetXForwarded()

			if clientIP, err := getIP(r.In, configVars.trustedProxies, configVars.clientIPHeader); err == nil {
				r.Out.Header.Set("X-Forwarded-For", clientIP)
			}
		},
	}

	http.HandleFunc("/health", healthHandler)

	http.HandleFunc("/", checkHandler(configVars.generalLimiter, configVars.authLimiter, slogger, proxy, configVars.authPaths, configVars.trustedProxies, configVars.clientIPHeader))
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+configVars.port, nil))
}
