package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner resolves its marketplace IDs from a mutable source on every
// call, proving the scheduler never captures a startup snapshot.
type fakeRunner struct {
	calls  atomic.Int32
	called chan struct{}
	ids    func() []string
	last   atomic.Value // []string
}

func (f *fakeRunner) RunOnce(ctx context.Context) {
	f.last.Store(f.ids())
	f.calls.Add(1)
	select {
	case f.called <- struct{}{}:
	default:
	}
}

func (f *fakeRunner) lastIDs() []string {
	v, _ := f.last.Load().([]string)
	return v
}

func TestSchedulerRunsImmediatelyThenOnInterval(t *testing.T) {
	current := []string{"wb"}
	runner := &fakeRunner{
		called: make(chan struct{}, 8),
		ids:    func() []string { return current },
	}
	s := New(runner, 5*time.Millisecond, testLogger())

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
	got := runner.lastIDs()
	if len(got) != 1 || got[0] != "wb" {
		t.Fatalf("expected marketplaces [wb], got %v", got)
	}
}

func TestSchedulerSeesMutatedMarketplacesBetweenTicks(t *testing.T) {
	var current atomic.Value // []string
	current.Store([]string{"wb"})
	runner := &fakeRunner{
		called: make(chan struct{}, 8),
		ids:    func() []string { v, _ := current.Load().([]string); return v },
	}
	s := New(runner, 5*time.Millisecond, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	<-runner.called // immediate run
	current.Store([]string{"ym", "ozon"})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-runner.called:
		case <-deadline:
			t.Fatal("second tick did not observe mutated marketplaces")
		}
		got := runner.lastIDs()
		if len(got) == 2 && got[0] == "ym" && got[1] == "ozon" {
			return
		}
	}
}

func TestSchedulerStopsBeforeFirstRunIfCancelled(t *testing.T) {
	runner := &fakeRunner{
		called: make(chan struct{}, 1),
		ids:    func() []string { return nil },
	}
	s := New(runner, time.Hour, testLogger())

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
