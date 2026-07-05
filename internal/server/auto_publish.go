package server

import (
	"context"
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

// StartAutoPublish keeps the static reviews-data export continuously fresh:
// every interval it republishes if curation, sync, remap, widget config or
// catalog changes marked the export dirty. interval <= 0 disables the loop.
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
				if published, err := s.runAutoPublishOnce(ctx); err != nil {
					s.logger.Warn("auto-publish failed", "error", err)
				} else if published {
					s.logger.Info("auto-publish: reviews-data regenerated")
				}
			}
		}
	}()
}
