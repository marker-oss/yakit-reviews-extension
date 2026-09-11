package server

import (
	"context"
	"reviews/internal/store"
	"time"
)

// autoRefreshCatalogOnce starts one incremental sitemap crawl if a sitemap is
// configured and no job is already running. Returns whether a crawl started.
func (s *Server) autoRefreshCatalogOnce(ctx context.Context) bool {
	sitemapURL := s.effectiveSitemapURL(ctx)
	if sitemapURL == "" {
		return false
	}
	if !s.tryStartSiteLinksRefresh() {
		return false
	}
	s.logger.Info("catalog auto-refresh started", "sitemap", sitemapURL)
	go s.runSiteLinksRefresh(store.TenantIDFromCtx(ctx), sitemapURL, false)
	return true
}

// StartCatalogAutoRefresh periodically re-crawls every tenant's shop
// sitemap in the background (incrementally) so new products reach
// links.json without the admin button. interval <= 0 disables the loop.
func (s *Server) StartCatalogAutoRefresh(ctx context.Context, interval time.Duration) {
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
				s.catalogRefreshTick(ctx)
			}
		}
	}()
}

// catalogRefreshTick starts one incremental crawl per tenant. The job slot
// is per-server; a tenant with no sitemap configured is skipped silently.
func (s *Server) catalogRefreshTick(ctx context.Context) {
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		s.logger.Warn("catalog refresh: list tenants failed", "error", err)
		return
	}
	for _, t := range tenants {
		tCtx := store.WithTenant(ctx, t.ID)
		if s.effectiveSitemapURL(tCtx) == "" {
			continue
		}
		s.autoRefreshCatalogOnce(tCtx)
	}
}
