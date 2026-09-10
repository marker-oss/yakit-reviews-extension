package server

import (
	"context"
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
	go s.runSiteLinksRefresh(sitemapURL, false)
	return true
}

// StartCatalogAutoRefresh periodically re-crawls the shop sitemap in the
// background (incrementally) so new products reach links.json without the
// admin button. interval <= 0 disables the loop.
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
				s.autoRefreshCatalogOnce(ctx)
			}
		}
	}()
}
