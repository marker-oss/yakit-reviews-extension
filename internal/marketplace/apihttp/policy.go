package apihttp

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter bounds accepted retry delays; anything larger is treated as
// an invalid header rather than trusted verbatim.
const maxRetryAfter = 15 * time.Minute

// Policy describes how one marketplace signals rate limiting: which status
// codes count as throttling, which header carries the retry delay, whether
// that header follows the standard RFC 9110 Retry-After format (seconds or
// HTTP-date) or a marketplace-specific integer-seconds header, and the
// minimum interval to enforce between requests regardless of throttling.
type Policy struct {
	id                 string
	throttleStatus     [2]int
	throttleStatusN    int
	retryHeader        string
	standardRetryAfter bool
	minInterval        time.Duration
}

func (p Policy) isThrottle(status int) bool {
	for i := 0; i < p.throttleStatusN; i++ {
		if p.throttleStatus[i] == status {
			return true
		}
	}
	return false
}

// throttle reports whether status/headers indicate this policy's throttle
// condition and, if so, the parsed retry delay. A delay of 0 with
// throttle==true means the header was missing, malformed, or out of bounds;
// callers must not retry automatically in that case.
func (p Policy) throttle(status int, headers http.Header, now time.Time) (bool, time.Duration) {
	if !p.isThrottle(status) {
		return false, 0
	}
	raw := strings.TrimSpace(headers.Get(p.retryHeader))
	if raw == "" {
		return true, 0
	}
	var delay time.Duration
	if p.standardRetryAfter {
		if seconds, err := strconv.Atoi(raw); err == nil {
			delay = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(raw); err == nil {
			delay = at.Sub(now)
		}
	} else if seconds, err := strconv.Atoi(raw); err == nil {
		delay = time.Duration(seconds) * time.Second
	}
	if delay <= 0 || delay > maxRetryAfter {
		return true, 0
	}
	return true, delay
}

// WBPolicy returns the Wildberries throttle policy: 429 with integer-seconds
// X-Ratelimit-Retry, plus a 350ms minimum interval between requests.
func WBPolicy() Policy {
	return Policy{
		id:                 "wb",
		throttleStatus:     [2]int{429, 0},
		throttleStatusN:    1,
		retryHeader:        "X-Ratelimit-Retry",
		standardRetryAfter: false,
		minInterval:        350 * time.Millisecond,
	}
}

// YMPolicy returns the Yandex Market throttle policy: 420 or 429 with a
// standard Retry-After header, no pre-emptive minimum interval.
func YMPolicy() Policy {
	return Policy{
		id:                 "ym",
		throttleStatus:     [2]int{420, 429},
		throttleStatusN:    2,
		retryHeader:        "Retry-After",
		standardRetryAfter: true,
		minInterval:        0,
	}
}

// OzonPolicy returns the Ozon throttle policy: 429 with a standard
// Retry-After header, no pre-emptive minimum interval.
func OzonPolicy() Policy {
	return Policy{
		id:                 "ozon",
		throttleStatus:     [2]int{429, 0},
		throttleStatusN:    1,
		retryHeader:        "Retry-After",
		standardRetryAfter: true,
		minInterval:        0,
	}
}
