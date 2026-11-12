# ratelimiter

**A production-style rate limiter written in Go, built as a standalone protective service and wired in front of Tren Den (a FastAPI workout tracker) as reverse-proxy middleware.**

Supports two rate-limiting algorithms - token bucket and sliding window counter - each with in-memory and Redis-backed implementations, atomic Lua-scripted operations, graceful degradation on Redis failure, and rate-limit response headers.
The algorithm is selectable at runtime via a single env var.

## Features
- Token bucket rate limiting (in-memory and Redis-backed)
- Sliding window counter rate limiting (in-memory and Redis-backed)
- Runtime-selectable algorithm (`ALGORITHM=token_bucket` or `sliding_window`) behind a shared `Limiter` interface
- Atomic Redis operations via Lua scripting (no race conditions under concurrent load)
- Fail-open behavior on Redis outage (configurable timeout + retry policy)
- Standard rate-limit headers (X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After)
- Reverse-proxy middleware mode for wiring into an existing backend
- Proxy-aware client IP resolution: trusted-proxy CIDRs, a right-to-left `X-Forwarded-For` walk, and an optional provider header such as `CF-Connecting-IP`
- The resolved client IP is forwarded to the backend
- Benchmarked against real Redis, sequentially and under concurrent (parallel) load

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

By default the service proxies requests to whatever `BACKEND_URL` points at, rate-limiting them before they reach your backend. The algorithm is chosen at startup via `ALGORITHM` (`token_bucket` or `sliding_window`, defaults to `token_bucket`) - only that algorithm's env vars are read, so the other algorithm's vars can be safely omitted from your environment (e.g. in a Render deployment).

### Configuration
 
| Env Var | Description | Example |
|---|---|---|
| `BACKEND_URL` | Backend the proxy forwards allowed requests to | `http://127.0.0.1:8000` |
| `REDIS_ADDR` | Redis connection address | `localhost:6379` |
| `PORT` | Port the ratelimiter listens on | `8080` |
| `ALGORITHM` | Which algorithm to use: `token_bucket` or `sliding_window` | `token_bucket` |
| `AUTH_PATHS` | Comma-separated list of exact paths routed to the auth limiter | `/login,/signup,/reset-password` |
| `TRUSTED_PROXIES` | Comma-separated CIDRs of proxies allowed to set client-IP headers. Empty = headers are ignored and the peer address is used | `127.0.0.1/32,::1/128` |
| `CLIENT_IP_HEADER` | Header holding the real client IP, checked before `X-Forwarded-For` (only read from a trusted peer) | `CF-Connecting-IP` |

**Token bucket** (read when `ALGORITHM=token_bucket`, or unset)

| Env Var | Description | Example |
|---|---|---|
| `MAX_TOKENS` | Bucket capacity for general requests | `10` |
| `REFILL_RATE` | Tokens refilled per second for general requests | `1` |
| `AUTH_MAX_TOKENS` | Bucket capacity for auth-path requests (`/login, /signup, or any other auth path`) | `5` |
| `AUTH_REFILL_RATE` | Tokens refilled per second for auth-path requests | `0.01` |
 
**Sliding window** (read when `ALGORITHM=sliding_window`)
| Env Var | Description | Example |
|---|---|---|
| `WINDOW_SIZE` | Window length in seconds for general requests | `60` |
| `LIMIT` | Max requests per window for general requests | `10` |
| `AUTH_WINDOW_SIZE` | Window length in seconds for auth-path requests | `60` |
| `AUTH_LIMIT` | Max requests per window for auth-path requests | `5` |

## Architecture
 
```
Client → ratelimiter (Go) → Backend (e.g. Tren Den)
              |
              └── Redis (rate-limit state, atomic via Lua)
```

In production on Render, requests arrive through Cloudflare and Render's edge proxy first (as observed from request headers):

```
Client → Cloudflare → Render edge → ratelimiter (Go) → Tren Den
```
 
- `tokenbucket/` - in-memory token bucket implementation
- `redisbucket/` - Redis-backed token bucket, atomic via `scripts/allow.lua`

- `slidingwindow/` - in-memory sliding window counter implementation
- `redisslidingwindow/` - Redis-backed sliding window counter, atomic via `scripts/allow.lua`

- `cmd/ratelimiter/` - HTTP server / reverse proxy entrypoint, plus `limiter.go` defining the shared `Limiter` interface both algorithms implement

## Design Decisions

### Two algorithms: token bucket vs. sliding window counter
Token bucket allows smooth, continuous refill and permits controlled bursts. Sliding window counter avoids the "double burst at window boundary" problem that plain fixed-window counting has, by blending a weighted fraction of the previous window's count into the current one - the weight decays linearly from 1.0 right after a window rolls over to 0.0 by the time it ends. This is an approximation (not as exact as a sliding log), but it's O(1) storage per client and cheap to compute, which is why it's a common choice in production rate limiters.

### Atomic operations via Lua scripting
 
Redis read-compute-write logic runs as a single atomic Lua script (`redisbucket/scripts/allow.lua`, `redisslidingwindow/scripts/allow.lua`) rather than as separate Redis calls. I ran a concurrency test and found out a race condition where a 25-token bucket allowed 10 extra requests (25 allowed instead of the expected limit) when hit with concurrent requests using naive check-then-decrement logic. Moving the logic into an atomic Lua script eliminated the overcount entirely, verified by the same test. Both algorithms share this pattern.
 
### Fail-open on Redis failure
 
If Redis is unreachable, the service fails **open**: requests are proxied through to the backend instead of being blocked, so a Redis outage doesn't take the whole backend down with it. `Allow()` uses a 200ms context timeout with reduced retries to fail fast instead of hanging. Trade-off: the backend is briefly unprotected during a Redis outage. I went with this decision since imho a rate limiter causing a full outage is a worse failure mode than a rate limiter briefly not rate-limiting. See `docs/design-decisions.md` for more detail.

### Client IP identification behind proxies

Keying buckets on `RemoteAddr` breaks behind a proxy: the peer is the proxy, not the client (on Render it was `[::1]` in every request I logged), so all users would share one bucket.

* Peer not in `TRUSTED_PROXIES`** - use the peer address and ignore all headers, so a direct client can't forge its identity.
    
* `CLIENT_IP_HEADER` set and holds a valid IP** - use it.
    
* Otherwise walk `X-Forwarded-For` right to left** and take the first entry that isn't a trusted proxy (the right end is what your own infrastructure appended; the left end is client-controlled). An unparseable entry falls back to the peer address.

On Render, a plain `X-Forwarded-For` walk returns a Cloudflare IP, so `CLIENT_IP_HEADER=CF-Connecting-IP` is used there; Cloudflare rejects forged values (error 1000). The resolved IP is forwarded to the backend as `X-Forwarded-For`.

Verified in production: one device drained its bucket while a phone on another network kept logging in normally. See `docs/design-decisions.md` for more detail.

## Benchmarks
 
| Benchmark | Token Bucket | Sliding Window Counter |
|---|---|---|
| `BenchmarkAllow` (miniredis) | ~148µs/op, 862 allocs/op | ~155µs/op, 874 allocs/op |
| `BenchmarkAllowRealRedis` (local Redis, sequential) | ~198µs/op, 23 allocs/op | ~205µs/op, 23 allocs/op |
| `BenchmarkAllowRealRedisParallel` (local Redis, concurrent, unique keys) | ~34µs/op, 25 allocs/op | ~36µs/op, 25 allocs/op |

* The miniredis benchmark shows a higher alloc count because miniredis rebuilds its Lua virtual environment on every call - that's a quirk of the test tool, not the real overhead
* Sliding window counter runs a small, consistent amount slower than token bucket sequentially (~3-5%), which tracks with it doing slightly more work per call (extra rollover check, extra weighted-average math, an extra field to encode/decode). Under parallel load with unique keys per client, this gap disappears - both are network/round-trip bound rather than compute-bound at that point, so the algorithmic difference gets swallowed by concurrency.
* Parallel benchmarks use a unique IP per request (via an atomic counter) so goroutines aren't contending for the same Redis key - this measures overall throughput rather than single-key lock contention.
* These numbers are all localhost-to-localhost. In production (Render), real network latency between the service and Redis will be higher than shown here.

## Known Limitations

- The limiter only protects what it fronts. On my Render setup the backend also has its own public URL, so a direct request skips the limiter. Making the backend a private service reachable only from the limiter closes that gap (not done yet).
- `CLIENT_IP_HEADER` is only as trustworthy as the proxy in front of the service. It's safe on Render because Cloudflare rejects a forged `CF-Connecting-IP`, but if the service were reachable without going through Cloudflare, a client could choose its own bucket.
- Buckets are per IP: users behind one NAT (office, campus) share a bucket, and an IPv6 client can rotate addresses within its /64 to get fresh buckets.

## Tech Stack

Go · Redis · Lua (embedded scripting)