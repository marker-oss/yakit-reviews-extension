package server

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"reviews/internal/store"
)

// tenantRateLimiter throttles public API requests per tenant (SaaS): one
// abusive widget must not starve the others. Sliding-window counters keyed
// by tenant id; static assets and admin routes are not rate-limited.
// ponytail: fixed per-tenant limits; per-plan limits arrive with billing.
type tenantRateLimiter struct {
	mu     sync.Mutex
	hits   map[uint][]time.Time
	perMin int
}

func newTenantRateLimiter(perMinute int) *tenantRateLimiter {
	if perMinute <= 0 {
		perMinute = 120
	}
	return &tenantRateLimiter{hits: map[uint][]time.Time{}, perMin: perMinute}
}

// allow records a hit and reports whether it fits in the window.
func (l *tenantRateLimiter) allow(tenantID uint, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-time.Minute)
	hits := filterSince(l.hits[tenantID], cutoff)
	if len(hits) >= l.perMin {
		l.hits[tenantID] = hits
		return false
	}
	l.hits[tenantID] = append(hits, now)
	// Prune piggyback: drop empty windows when the map grows large.
	if len(l.hits) > 4096 {
		for id, h := range l.hits {
			if len(filterSince(h, cutoff)) == 0 {
				delete(l.hits, id)
			}
		}
	}
	return true
}

// tenantRateLimit throttles keyed public data routes per tenant. Mounted in
// the handler chain after tenantScope; tenantless requests (compat mode,
// health, statics, admin session routes) pass through untouched.
func (s *Server) tenantRateLimit(next http.Handler) http.Handler {
	if s.tenantLimiter == nil {
		s.tenantLimiter = newTenantRateLimiter(120)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := requestTenantID(r); ok && !s.tenantLimiter.allow(id, time.Now()) {
			writeError(w, http.StatusTooManyRequests, errors.New("слишком много запросов, попробуйте позже"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestTenantID extracts the tenant stamped by tenantScope without the
// strict-mode panic: tenantless requests report ok=false.
func requestTenantID(r *http.Request) (uint, bool) {
	id, ok := store.TenantIDFromCtxSafe(r.Context())
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}
