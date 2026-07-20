package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
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

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 1, 1)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, slogger)


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

	rdb := redisbucket.NewRedisBucket("1.1.1.1", 1, 1)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	req.RemoteAddr = "a-bad-ip-address"

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, slogger)

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCheckHandlerRedisError(t *testing.T) {

	mr, err := miniredis.Run()

	if err != nil {
		t.Fatal(err)
	}

	rdb := redisbucket.NewRedisBucket(mr.Addr(), 1, 1)
	mr.Close()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	handler := checkHandler(rdb, slogger)


	handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}