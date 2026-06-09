package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRunner struct {
	calls   atomic.Int32
	called  chan struct{}
	lastIDs []string
}

func (f *fakeRunner) RunOnce(ctx context.Context, marketplaces []string) {
	f.lastIDs = marketplaces
	f.calls.Add(1)
	select {
	case f.called <- struct{}{}:
	default:
	}
}

func TestSchedulerRunsImmediatelyThenOnInterval(t *testing.T) {
	runner := &fakeRunner{called: make(chan struct{}, 8)}
	s := New(runner, 5*time.Millisecond, []string{"wb"}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Immediate first run.
	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run immediately")
	}
	// At least one interval-triggered run.
	select {
	case <-runner.called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run on interval")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop on context cancel")
	}

	if runner.calls.Load() < 2 {
		t.Fatalf("expected at least 2 runs, got %d", runner.calls.Load())
	}
	if len(runner.lastIDs) != 1 || runner.lastIDs[0] != "wb" {
		t.Fatalf("expected marketplaces [wb], got %v", runner.lastIDs)
	}
}

func TestSchedulerStopsBeforeFirstRunIfCancelled(t *testing.T) {
	runner := &fakeRunner{called: make(chan struct{}, 1)}
	s := New(runner, time.Hour, []string{"wb"}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run.

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not return on pre-cancelled context")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
