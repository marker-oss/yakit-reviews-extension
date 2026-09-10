package syncer

import (
	"sync"
	"testing"
)

func TestTryAcquireBlocksSecondCallUntilRelease(t *testing.T) {
	c := NewCoordinator()

	release, ok := c.TryAcquire("wb")
	if !ok {
		t.Fatalf("first TryAcquire(wb) = false, want true")
	}

	if _, ok := c.TryAcquire("wb"); ok {
		t.Fatalf("second TryAcquire(wb) while busy = true, want false")
	}

	release()

	if _, ok := c.TryAcquire("wb"); !ok {
		t.Fatalf("TryAcquire(wb) after release = false, want true")
	}
}

func TestTryAcquireIsPerMarketplace(t *testing.T) {
	c := NewCoordinator()

	if _, ok := c.TryAcquire("wb"); !ok {
		t.Fatalf("TryAcquire(wb) = false, want true")
	}
	if _, ok := c.TryAcquire("ym"); !ok {
		t.Fatalf("TryAcquire(ym) = false, want true")
	}
}

func TestBusyReflectsAcquisitionState(t *testing.T) {
	c := NewCoordinator()

	if c.Busy("wb") {
		t.Fatalf("Busy(wb) before acquire = true, want false")
	}

	release, ok := c.TryAcquire("wb")
	if !ok {
		t.Fatalf("TryAcquire(wb) = false, want true")
	}
	if !c.Busy("wb") {
		t.Fatalf("Busy(wb) after acquire = false, want true")
	}

	release()

	if c.Busy("wb") {
		t.Fatalf("Busy(wb) after release = true, want false")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	c := NewCoordinator()

	release, ok := c.TryAcquire("wb")
	if !ok {
		t.Fatalf("TryAcquire(wb) = false, want true")
	}

	release()
	release() // must not panic or double-free another holder's acquisition

	if _, ok := c.TryAcquire("wb"); !ok {
		t.Fatalf("TryAcquire(wb) after double release = false, want true")
	}
}

func TestTryAcquireRaceExactlyOneWinner(t *testing.T) {
	c := NewCoordinator()

	const n = 100
	var wg sync.WaitGroup
	var successes int32
	var mu sync.Mutex
	var release func()

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if r, ok := c.TryAcquire("wb"); ok {
				mu.Lock()
				successes++
				release = r
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes)
	}

	release()

	if _, ok := c.TryAcquire("wb"); !ok {
		t.Fatalf("TryAcquire(wb) after race winner released = false, want true")
	}
}
