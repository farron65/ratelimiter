package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/farron65/ratelimiter/redisbucket"
)

func TestCheckHandlerTooManyRequests(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	defer mr.Close()

	rdb := redisbucket.NewRedisBucket("testing", mr.Addr(), 1, 1)
	rdb2 := redisbucket.NewRedisBucket("testing2", mr.Addr(), 1, 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer backend.Close()

	target, err := url.Parse(backend.URL)

	if err != nil {
		t.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, rdb2, slogger, proxy)


	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, rec.Code)
	}
}

func TestCheckHandlerBadIP(t *testing.T) {

	rdb := redisbucket.NewRedisBucket("testing", "1.1.1.1", 1, 1)
	rdb2 := redisbucket.NewRedisBucket("testing2", "1.1.1.1", 1, 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer backend.Close()

	target, err := url.Parse(backend.URL)

	if err != nil {
		t.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	req.RemoteAddr = "a-bad-ip-address"

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, rdb2, slogger, proxy)

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRedisDownButRequestSucceeds(t *testing.T) {

	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}
	
	rdb := redisbucket.NewRedisBucket("testing", mr.Addr(), 1, 1)
	rdb2 := redisbucket.NewRedisBucket("testing2", mr.Addr(), 1, 1)

	mr.Close() // Close redis, since we are try to imitate the scenario when redis is down

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer backend.Close()

	target, err := url.Parse(backend.URL)

	if err != nil {
		t.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, rdb2, slogger, proxy)

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}