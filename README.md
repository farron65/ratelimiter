# ratelimiter

**A production-style rate limiter written in Go, built as a standalone protective service and wired in front of Tren Den (a FastAPI workout tracker) as reverse-proxy middleware.**

Supports in-memory and Redis-backed token bucket algorithm, with atomic Lua-scripted operations, graceful degradation on Redis failure, and rate-limit response headers.

## Features
- Token bucket rate limiting (in-memory and Redis-backed)
- Atomic Redis operations via Lua scripting (no race conditions under concurrent load)
- Fail-open behavior on Redis outage (configurable timeout + retry policy)
- Standard rate-limit headers (X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After)
- Reverse-proxy middleware mode for wiring into an existing backend
- Benchmarked against real Redis (~205µs/op)

## Quickstart
```bash
git clone https://github.com/farron65/ratelimiter.git
cd ratelimiter
cp .env.example .env
# edit .env with your Redis connection details

go run cmd/ratelimiter/main.go
```
> **Note:** Start Redis before running the service (`redis-server`, or `docker run -p 6379:6379 redis`). If Redis isn't running, `REDIS_ADDR` will be unreachable and requests will silently succeed unrated (fail-open behavior) instead of erroring - so it can look like everything's working when nothing is actually being rate-limited.

> Your backend (`BACKEND_URL`) also needs to be running, or the proxy has nothing to forward allowed requests to.

By default the service proxies requests to whatever `BACKEND_URL` points at, rate-limiting them before they reach your backend.
 
### Configuration
 
| Env Var | Description | Example |
|---|---|---|
| `BACKEND_URL` | Backend the proxy forwards allowed requests to | `http://127.0.0.1:8000` |
| `MAX_TOKENS` | Bucket capacity for general requests | `10` |
| `REFILL_RATE` | Tokens refilled per second for general requests | `1` |
| `AUTH_PATHS` | Comma-separated list of exact paths routed to the auth bucket | `/login,/signup,/reset-password` |
| `AUTH_MAX_TOKENS` | Bucket capacity for auth-path requests (`/login, /signup, or any other auth path`) | `5` |
| `AUTH_REFILL_RATE` | Tokens refilled per second for auth-path requests | `0.01` |
 
## Architecture
 
```
Client → ratelimiter (Go) → Backend (e.g. Tren Den)
              |
              └── Redis (token state, atomic via Lua)
```
 
- `tokenbucket/` - in-memory token bucket implementation
- `redisbucket/` - Redis-backed token bucket, atomic via `scripts/allow.lua`
- `cmd/ratelimiter/` - HTTP server / reverse proxy entrypoint
## Design Decisions
 
### Atomic operations via Lua scripting
 
Redis read-refill-decrement-write logic runs as a single atomic Lua script (`redisbucket/scripts/allow.lua`) rather than as separate Redis calls. I ran a concurrency test and found out a race condition where a 25-token bucket allowed 10 extra requests (25 allowed instead of the expected limit) when hit with concurrent requests using naive check-then-decrement logic. Moving the logic into an atomic Lua script eliminated the overcount entirely, verified by the same test.
 
### Fail-open on Redis failure
 
If Redis is unreachable, the service fails **open** - requests are proxied through to the backend rather than blocked - rather than failing closed and taking the whole backend down with it. `Allow()` uses a 200ms context timeout with reduced retries to fail fast rather than hang. Trade-off: the backend is briefly unprotected during a Redis outage. This was accepted deliberately since a rate limiter causing a full outage is a worse failure mode than a rate limiter briefly not rate-limiting. See `docs/design-decisions.md` for more detail.
 
## Benchmarks
 
| Benchmark | Result |
|---|---|
| `BenchmarkAllow` (miniredis) | ~148µs/op, 862 allocs/op |
| `BenchmarkAllowRealRedis` (local Redis) | ~205µs/op, 23 allocs/op |

* The miniredis benchmark shows a higher alloc count because miniredis rebuilds its Lua virtual environment on every call - that's a quirk of the test tool, not the real overhead. Against actual Redis (BenchmarkAllowRealRedis), allocations drop to 23 allocs/op.
 
Verified via `PoolStats`: single dial, no reconnect churn under load.
 
## Known Limitations

- No `X-Forwarded-For` support yet (not currently needed by Tren Den my backend service)

## Next Steps
- [ ] Sliding window rate-limiting algorithm (second algorithm alongside token bucket)

## Tech Stack

Go · Redis · Lua (embedded scripting)