package server

import (
	"context"
	"reviews/internal/store"
	"time"
)

// runAutoPublishOnce regenerates the static export when data changed since
// the last publish. Returns whether a publish happened.
func (s *Server) runAutoPublishOnce(ctx context.Context) (bool, error) {
	dirtyAt, dirty, err := s.store.ExportDirtySince(ctx)
	if err != nil || !dirty {
		return false, err
	}
	if _, err := s.publishReviewsData(ctx); err != nil {
		return false, err
	}
	// Publish covers changes up to the observed dirty mark only: anything that
	// landed mid-export keeps the store dirty for the next tick.
	if err := s.store.MarkExportPublished(ctx, dirtyAt); err != nil {
		return false, err
	}
	return true, nil
}

// StartAutoPublish keeps the static reviews-data export continuously fresh
// for every tenant: each tick walks all tenants and republishes the ones
// whose data changed since their last publish. interval <= 0 disables the
// loop.
func (s *Server) StartAutoPublish(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.autoPublishTick(ctx)
			}
		}
	}()
}

// autoPublishTick runs runAutoPublishOnce for every tenant. A failing tenant
// logs and yields; one broken tenant never starves the others.
func (s *Server) autoPublishTick(ctx context.Context) {
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		s.logger.Warn("auto-publish: list tenants failed", "error", err)
		return
	}
	for _, t := range tenants {
		published, err := s.runAutoPublishOnce(store.WithTenant(ctx, t.ID))
		if err != nil {
			s.logger.Warn("auto-publish failed", "tenant", t.ID, "error", err)
		} else if published {
			s.logger.Info("auto-publish: reviews-data regenerated", "tenant", t.ID)
		}
	}
}
