package server

import (
	"context"
	"time"

	"reviews/internal/store"
)

// StartTrialExpiry pauses trial tenants whose window has ended. Runs hourly;
// the store method is idempotent so frequent ticks are harmless. Disabled in
// single-tenant (compat) mode: the implicit default tenant has no billing
// lifecycle and existing installs may carry a zero trial_ends_at.
func (s *Server) StartTrialExpiry(ctx context.Context) {
	if !store.StrictTenantMode() {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := s.store.PauseExpiredTrials(ctx); err != nil {
					s.logger.Warn("trial expiry failed", "error", err)
				} else if n > 0 {
					s.logger.Info("trial expired: tenants paused", "count", n)
				}
			}
		}
	}()
}
