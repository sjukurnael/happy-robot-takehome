package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Write-rate limits. A human clicking around never gets near these; a
// runaway client (say, a PATCH loop from a buggy effect) does, and matters
// here specifically because every mutation (a) takes the project row lock
// that serializes all writers to that project and (b) fans out to every
// connected WebSocket — so one spammy client's cost is multiplied by
// everyone else's connection count.
const (
	mutationRatePerSec = 15
	mutationBurst      = 30
	// Idle buckets are dropped after this long so the per-key map can't
	// grow without bound.
	bucketIdleTTL = 10 * time.Minute
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{buckets: map[string]*bucket{}}
	go rl.sweep()
	return rl
}

func (rl *rateLimiter) sweep() {
	for range time.Tick(bucketIdleTTL) {
		cutoff := time.Now().Add(-bucketIdleTTL)
		rl.mu.Lock()
		for key, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(mutationRatePerSec, mutationBurst)}
		rl.buckets[key] = b
	}
	b.lastSeen = time.Now()
	return b.limiter.Allow()
}

// clientKey identifies the caller for rate-limiting purposes. The first
// X-Forwarded-For hop is preferred because in the Dockerized deployment
// every request reaches Go through nginx, so RemoteAddr is always the
// proxy — trusting the header is fine here since nginx (which sets it) is
// the only way in. Falls back to RemoteAddr for direct/native-dev access.
func clientKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware applies a per-caller token bucket to mutating
// requests only — reads stay unthrottled (they're cheap, cacheable, and
// gap-filling depends on them). Over-limit requests get a 429 with the
// standard error shape, which the frontend already surfaces via ApiError.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			if !rl.allow(clientKey(r)) {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded — slow down")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
