package apihttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// --- Step 5: Executor retry/sanitization tests ---

func TestExecutorReadRetries(t *testing.T) {
	var calls atomic.Int32
	var factoryCalls atomic.Int32

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		if n == 1 {
			return &http.Response{
				StatusCode: 429,
				Header:     http.Header{"X-Ratelimit-Retry": {"1"}},
				Body:       io.NopCloser(strings.NewReader("throttled")),
			}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }

	client := &http.Client{Transport: transport}

	resp, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		factoryCalls.Add(1)
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks?token=secret", nil)
		return req, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("transport calls = %d, want 2", got)
	}
	if got := factoryCalls.Load(); got != 2 {
		t.Fatalf("factory calls = %d, want 2", got)
	}
}

func TestExecutorWriteNoRetry(t *testing.T) {
	var calls atomic.Int32

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: 429,
			Header:     http.Header{"X-Ratelimit-Retry": {"5"}},
			Body:       io.NopCloser(strings.NewReader("throttled")),
		}, nil
	})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }
	client := &http.Client{Transport: transport}

	_, err := e.Do(context.Background(), client, WBPolicy(), Write, func(ctx context.Context) (*http.Request, error) {
		req, _ := http.NewRequestWithContext(ctx, "PATCH", "http://example.com/api/v1/feedbacks", strings.NewReader(`{"text":"reply"}`))
		return req, nil
	})
	if err == nil {
		t.Fatal("expected error for throttled write")
	}
	var te *ThrottleError
	if !errors.As(err, &te) {
		t.Fatalf("error type = %T, want *ThrottleError", err)
	}
	if te.RetryAfter != 5*time.Second {
		t.Fatalf("RetryAfter = %v, want 5s", te.RetryAfter)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want 1", got)
	}
}

func TestExecutorMissingRetryHeader(t *testing.T) {
	var calls atomic.Int32

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: 429,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("throttled")),
		}, nil
	})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }
	client := &http.Client{Transport: transport}

	_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
		return req, nil
	})
	if err == nil {
		t.Fatal("expected error for missing retry header")
	}
	var te *ThrottleError
	if !errors.As(err, &te) {
		t.Fatalf("error type = %T, want *ThrottleError", err)
	}
	if te.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0", te.RetryAfter)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want 1", got)
	}
}

func TestExecutorReadFreshBody(t *testing.T) {
	var bodies []string

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			return &http.Response{
				StatusCode: 429,
				Header:     http.Header{"X-Ratelimit-Retry": {"1"}},
				Body:       io.NopCloser(strings.NewReader("throttled")),
			}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }
	client := &http.Client{Transport: transport}

	_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		body := bytes.NewReader([]byte(`{"page":1}`))
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://example.com/api/v1/feedbacks", body)
		return req, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("got %d bodies, want 2", len(bodies))
	}
	for i, b := range bodies {
		if b != `{"page":1}` {
			t.Fatalf("body[%d] = %q, want %q", i, b, `{"page":1}`)
		}
	}
}

func TestThrottleErrorSanitization(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 429,
			Header:     http.Header{"X-Ratelimit-Retry": {"60"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate_limit","secret":"s3cr3t"}`)),
		}, nil
	})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }
	client := &http.Client{Transport: transport}

	_, err := e.Do(context.Background(), client, WBPolicy(), Write, func(ctx context.Context) (*http.Request, error) {
		req, _ := http.NewRequestWithContext(ctx, "PATCH", "http://example.com/api/v1/feedbacks?token=secret", strings.NewReader(`{"text":"reply"}`))
		req.Header.Set("Authorization", "Bearer tok123")
		return req, nil
	})
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()

	// Must include marketplace name, path, status, and delay
	for _, want := range []string{"Wildberries", "/api/v1/feedbacks", "429"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() missing %q: %s", want, msg)
		}
	}

	// Must NOT include secrets, query strings, auth headers, or response body
	for _, bad := range []string{"?token=secret", "token=secret", "Authorization", "Bearer", "tok123", "s3cr3t", `"error"`, "rate_limit"} {
		if strings.Contains(msg, bad) {
			t.Errorf("Error() leaks %q: %s", bad, msg)
		}
	}
}

// --- Step 6: Executor concurrency/cancellation tests ---

func TestExecutorWBSerializes(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	release := make(chan struct{})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		cur := active.Add(1)
		for {
			old := maxActive.Load()
			if cur <= old {
				break
			}
			if maxActive.CompareAndSwap(old, cur) {
				break
			}
		}
		// Hold the transport until every goroutine has entered; with proper
		// per-marketplace serialization only one is ever here at a time.
		<-release
		active.Add(-1)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	client := &http.Client{Transport: transport}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
				return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(release)
	wg.Wait()

	if got := maxActive.Load(); got != 1 {
		t.Fatalf("maxActive = %d, want 1 (WB should serialize)", got)
	}
}

func TestExecutorCrossMarketplaceOverlaps(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})

	e := NewExecutor()
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		cur := active.Add(1)
		for {
			old := maxActive.Load()
			if cur <= old {
				break
			}
			if maxActive.CompareAndSwap(old, cur) {
				break
			}
		}
		// Signal arrival and wait for release
		arrived <- struct{}{}
		<-release
		active.Add(-1)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	client := &http.Client{Transport: transport}
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
		})
		if err != nil {
			t.Errorf("wb: unexpected error: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := e.Do(context.Background(), client, YMPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "GET", "http://example.com/campaigns/123/feedback", nil)
		})
		if err != nil {
			t.Errorf("ym: unexpected error: %v", err)
		}
	}()

	// Wait for both to arrive in the transport
	<-arrived
	<-arrived
	// Release both
	close(release)

	wg.Wait()

	if got := maxActive.Load(); got != 2 {
		t.Fatalf("maxActive = %d, want 2 (WB+YM should overlap)", got)
	}
}

func TestExecutorWBMinInterval(t *testing.T) {
	var attempts []time.Time
	var mu sync.Mutex

	fakeNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var fakeClock sync.Mutex

	e := NewExecutor()
	e.now = func() time.Time {
		fakeClock.Lock()
		defer fakeClock.Unlock()
		return fakeNow
	}
	e.wait = func(_ context.Context, d time.Duration) error {
		fakeClock.Lock()
		fakeNow = fakeNow.Add(d)
		fakeClock.Unlock()
		return nil
	}

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		attempts = append(attempts, e.now())
		mu.Unlock()
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	client := &http.Client{Transport: transport}

	// First call
	_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
	})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call — should be delayed by at least 350ms
	_, err = e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if len(attempts) < 2 {
		t.Fatalf("got %d attempts, want >= 2", len(attempts))
	}
	gap := attempts[1].Sub(attempts[0])
	if gap < 350*time.Millisecond {
		t.Fatalf("second attempt started %v after first, want >= 350ms", gap)
	}
}

func TestExecutorCancellationDuringWait(t *testing.T) {
	transportCalled := make(chan struct{}, 10)
	e := NewExecutor()
	waitStarted := make(chan struct{})
	e.wait = func(ctx context.Context, d time.Duration) error {
		close(waitStarted)
		<-ctx.Done()
		return ctx.Err()
	}

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalled <- struct{}{}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	client := &http.Client{Transport: transport}

	// First call to acquire the marketplace lock
	_, err := e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
	})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Drain channel from the first call's transport hit
	<-transportCalled

	// Second call with cancellable context — will block on minInterval wait
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := e.Do(ctx, client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
		})
		errCh <- err
	}()

	// Wait until the executor reports it is pacing (wait hook invoked), then cancel.
	<-waitStarted
	cancel()

	err = <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	// Ensure transport was not called for the cancelled request
	select {
	case <-transportCalled:
		t.Fatal("transport should not be called after cancellation")
	default:
	}

	// Ensure the marketplace lock is released — a third call should work
	e.wait = func(_ context.Context, _ time.Duration) error { return nil }
	_, err = e.Do(context.Background(), client, WBPolicy(), Read, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "GET", "http://example.com/api/v1/feedbacks", nil)
	})
	if err != nil {
		t.Fatalf("third call (after cancel): %v", err)
	}
}

func TestThrottleErrorFormatting(t *testing.T) {
	te := &ThrottleError{
		Marketplace: "wb",
		Path:        "/api/v1/feedbacks",
		Status:      429,
		RetryAfter:  time.Minute,
	}
	msg := te.Error()
	if !strings.Contains(msg, "Wildberries") {
		t.Errorf("missing marketplace name: %s", msg)
	}
	if !strings.Contains(msg, "/api/v1/feedbacks") {
		t.Errorf("missing path: %s", msg)
	}
	if !strings.Contains(msg, "429") {
		t.Errorf("missing status: %s", msg)
	}
	if !strings.Contains(msg, fmt.Sprint(time.Minute)) {
		t.Errorf("missing duration: %s", msg)
	}
}
