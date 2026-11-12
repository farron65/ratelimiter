# Design Decisions

## Redis failure handling: fail-open

**Decision:** If Redis is unreachable or returns an error, the rate limiter
lets the request through to the backend instead of blocking it.

**Context:** The rate limiter sits in front of (Tren Den) the backend as a protective
layer, not as the core service. If Redis goes down and the limiter fails
closed (rejects all requests), a minor infra issue with Redis turns into a
full outage of the actual product - which is a worse failure than
temporarily having no rate limiting.

**Implementation:**
- `Allow()` uses a 200ms context timeout on the Redis call, so a Redis
  outage is detected quickly instead of hanging on the client's default
  retry/dial behavior (which was observed to take up to ~1.7s per request).
- `MaxRetries` on the Redis client is reduced to 2, since retries only help
  with transient blips - a persistent outage isn't fixed by retrying more,
  it just adds latency for no benefit.
- On a Redis error, `checkHandler` logs the failure (`slog.Error`) so it's
  visible in monitoring, but proxies the request through unrated rather
  than returning a 500.

**Tradeoff:** During a Redis outage, the backend is temporarily
unprotected from abusive traffic. This is accepted as the better failure
mode for this project - availability of (Tren Den) the backend service is prioritized over
strict rate limiting, since the limiter's job is to protect the backend
under normal conditions, not to be a hard dependency for every request.