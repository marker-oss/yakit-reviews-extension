// Package syncer coordinates concurrent sync runs per marketplace so that
// only one sync operation is in flight for a given marketplace at a time.
package syncer

import "sync"

// Coordinator tracks which marketplaces currently have an in-flight sync.
type Coordinator struct {
	mu   sync.Mutex
	busy map[string]bool
}

// NewCoordinator returns a ready-to-use Coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{busy: make(map[string]bool)}
}

// TryAcquire attempts to claim the sync slot for marketplace. If ok is true,
// the caller owns the slot until it calls release; release is safe to call
// more than once.
func (c *Coordinator) TryAcquire(marketplace string) (release func(), ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.busy[marketplace] {
		return nil, false
	}
	c.busy[marketplace] = true

	var once sync.Once
	release = func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			delete(c.busy, marketplace)
		})
	}
	return release, true
}

// Busy reports whether marketplace currently has an in-flight sync.
func (c *Coordinator) Busy(marketplace string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy[marketplace]
}
