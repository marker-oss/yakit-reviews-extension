package apihttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Operation classifies a marketplace API call for retry decisions.
type Operation uint8

const (
	// Read marks idempotent calls that may be retried once on throttle.
	Read Operation = iota
	// Write marks non-idempotent calls that must never be retried.
	Write
)

// RequestFactory builds a fresh *http.Request for each attempt. A new body
// must be constructed on every call; reusing a consumed body is a bug.
type RequestFactory func(context.Context) (*http.Request, error)

// ThrottleError reports a marketplace rate-limit response. Fields carry
// loggable metadata; Error() returns user-facing Russian text.
type ThrottleError struct {
	Marketplace string
	Path        string
	Status      int
	RetryAfter  time.Duration
}

var marketplaceNames = map[string]string{
	"wb":   "Wildberries",
	"ym":   "Yandex.Market",
	"ozon": "Ozon",
}

func (e *ThrottleError) Error() string {
	name := e.Marketplace
	if n, ok := marketplaceNames[e.Marketplace]; ok {
		name = n
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s временно ограничил запросы (HTTP %d, %s). Повторите через %s.",
			name, e.Status, e.Path, e.RetryAfter)
	}
	return fmt.Sprintf("%s временно ограничил запросы (HTTP %d, %s).",
		name, e.Status, e.Path)
}

// marketplaceState serializes and paces requests to a single marketplace.
type marketplaceState struct {
	mu          sync.Mutex
	lastAttempt time.Time
}

// Executor routes marketplace API requests through per-marketplace
// serialization and rate-limit handling. One instance must be shared across
// every marketplace adapter in the process.
type Executor struct {
	mu     sync.Mutex
	states map[string]*marketplaceState
	now    func() time.Time                           // test hook
	wait   func(context.Context, time.Duration) error // test hook
}

// NewExecutor creates a ready-to-use executor with production clock and wait.
func NewExecutor() *Executor {
	return &Executor{
		states: make(map[string]*marketplaceState),
		now:    time.Now,
		wait:   productionWait,
	}
}

func productionWait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Executor) getState(id string) *marketplaceState {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[id]
	if s == nil {
		s = &marketplaceState{}
		e.states[id] = s
	}
	return s
}

// Do executes one marketplace request through the policy's rate-limit rules.
// It serializes requests per marketplace, enforces minimum intervals, retries
// reads once on valid throttle headers, and never retries writes.
func (e *Executor) Do(ctx context.Context, client *http.Client, policy Policy, operation Operation, makeRequest RequestFactory) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}

	state := e.getState(policy.id)
	state.mu.Lock()
	defer state.mu.Unlock()

	// Enforce minimum interval.
	if policy.minInterval > 0 {
		now := e.now()
		elapsed := now.Sub(state.lastAttempt)
		if elapsed < policy.minInterval {
			if err := e.wait(ctx, policy.minInterval-elapsed); err != nil {
				return nil, err
			}
		}
	}

	state.lastAttempt = e.now()

	// First attempt.
	resp, path, err := e.attempt(ctx, client, makeRequest)
	if err != nil {
		return nil, err
	}

	throttled, delay := policy.throttle(resp.StatusCode, resp.Header, e.now())
	if !throttled {
		return resp, nil
	}

	// Throttled — drain and close the first response body.
	drainAndClose(resp.Body)

	// Write or invalid delay or Read with no delay: return ThrottleError.
	if operation == Write || delay <= 0 {
		return nil, &ThrottleError{
			Marketplace: policy.id,
			Path:        path,
			Status:      resp.StatusCode,
			RetryAfter:  delay,
		}
	}

	// Read with valid delay: wait and retry once.
	if err := e.wait(ctx, delay); err != nil {
		return nil, err
	}

	state.lastAttempt = e.now()

	resp2, path2, err := e.attempt(ctx, client, makeRequest)
	if err != nil {
		return nil, err
	}

	throttled2, delay2 := policy.throttle(resp2.StatusCode, resp2.Header, e.now())
	if !throttled2 {
		return resp2, nil
	}

	// Second throttle on read: give up.
	drainAndClose(resp2.Body)
	return nil, &ThrottleError{
		Marketplace: policy.id,
		Path:        path2,
		Status:      resp2.StatusCode,
		RetryAfter:  delay2,
	}
}

// attempt builds a fresh request via the factory and executes it.
// Returns the response and the sanitized URL path (without query string).
func (e *Executor) attempt(ctx context.Context, client *http.Client, makeRequest RequestFactory) (*http.Response, string, error) {
	req, err := makeRequest(ctx)
	if err != nil {
		return nil, "", err
	}
	// Capture only the path for error messages; never query or full URL.
	path := req.URL.Path
	resp, err := client.Do(req)
	if err != nil {
		return nil, path, err
	}
	return resp, path, nil
}

// drainAndClose drains at most 4 KiB from a response body and closes it.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	io.CopyN(io.Discard, body, 4096)
	body.Close()
}
