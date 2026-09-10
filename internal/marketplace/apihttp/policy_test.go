package apihttp

import (
	"net/http"
	"testing"
	"time"
)

func TestPolicies(t *testing.T) {
	tests := []struct {
		name         string
		policy       Policy
		status       int
		header       http.Header
		wantThrottle bool
		wantDelay    time.Duration
	}{
		{"wb retry seconds", WBPolicy(), 429, http.Header{"X-Ratelimit-Retry": {"696"}}, true, 696 * time.Second},
		{"wb other status", WBPolicy(), 500, nil, false, 0},
		{"ym 420 retry-after", YMPolicy(), 420, http.Header{"Retry-After": {"3"}}, true, 3 * time.Second},
		{"ym 429 http date", YMPolicy(), 429, http.Header{"Retry-After": {time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)}}, true, time.Minute},
		{"ozon 429 retry-after", OzonPolicy(), 429, http.Header{"Retry-After": {"2"}}, true, 2 * time.Second},
		{"missing header", OzonPolicy(), 429, nil, true, 0},
		{"over max", WBPolicy(), 429, http.Header{"X-Ratelimit-Retry": {"901"}}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotThrottle, gotDelay := tt.policy.throttle(tt.status, tt.header, time.Now())
			if gotThrottle != tt.wantThrottle {
				t.Fatalf("throttle = %v, want %v", gotThrottle, tt.wantThrottle)
			}
			diff := gotDelay - tt.wantDelay
			if diff < 0 {
				diff = -diff
			}
			if diff > time.Second {
				t.Fatalf("delay = %v, want %v (±1s tolerance)", gotDelay, tt.wantDelay)
			}
		})
	}
}

func TestPolicyMinIntervals(t *testing.T) {
	if got := WBPolicy().minInterval; got != 350*time.Millisecond {
		t.Fatalf("WBPolicy().minInterval = %v, want 350ms", got)
	}
	if got := YMPolicy().minInterval; got != 0 {
		t.Fatalf("YMPolicy().minInterval = %v, want 0", got)
	}
	if got := OzonPolicy().minInterval; got != 0 {
		t.Fatalf("OzonPolicy().minInterval = %v, want 0", got)
	}
}

func TestPolicyRejectsInvalidDelay(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
	}{
		{"zero", http.Header{"X-Ratelimit-Retry": {"0"}}},
		{"negative", http.Header{"X-Ratelimit-Retry": {"-5"}}},
		{"malformed", http.Header{"X-Ratelimit-Retry": {"soon"}}},
		{"above max", http.Header{"X-Ratelimit-Retry": {"901"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			throttle, delay := WBPolicy().throttle(429, tt.header, time.Now())
			if !throttle {
				t.Fatalf("throttle = false, want true")
			}
			if delay != 0 {
				t.Fatalf("delay = %v, want 0", delay)
			}
		})
	}
}
