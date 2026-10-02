package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit implements a fixed-window counter per key (e.g. IP or user ID):
// INCR a Redis counter for the current window, set its TTL to the window
// length on first increment, and reject once the limit is exceeded.
//
// Fixed-window is simpler than a sliding window or token bucket and is
// good enough for login/checkout abuse protection. It allows a burst of
// up to 2x the limit right at a window boundary — acceptable here; the
// Flash Sale system in Phase 4 uses a stricter per-second sliding window
// specifically because that boundary burst matters at that scale.
//
// keyFunc extracts the rate-limit key from the request (IP for anonymous
// endpoints like login, user ID for authenticated ones like checkout).
func RateLimit(client *redis.Client, limit int, window time.Duration, prefix string, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := fmt.Sprintf("ratelimit:%s:%s", prefix, keyFunc(r))
			ctx := context.Background()

			count, err := client.Incr(ctx, key).Result()
			if err != nil {
				// Fail open: a Redis outage shouldn't take down checkout/login entirely.
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				client.Expire(ctx, key, window)
			}
			if count > int64(limit) {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", window.Seconds()))
				http.Error(w, `{"error":"rate limit exceeded, try again shortly"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extracts a best-effort client IP for use as a rate-limit key
// on unauthenticated endpoints. Good enough behind a single reverse proxy;
// Phase 5's CDN/WAF layer handles more sophisticated IP resolution.
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}
