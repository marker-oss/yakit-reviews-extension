package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	staticexport "reviews/internal/export"
	"reviews/internal/reviewjson"
	"reviews/internal/site"
)

// The whole crawl gets a generous ceiling (a 4500-product shop takes about ten
// minutes), each page fetch a short one so a hung connection cannot stall a
// worker forever.
const (
	siteLinksJobTimeout    = 45 * time.Minute
	siteLinksExportTimeout = 2 * time.Minute
	siteLinksFetchTimeout  = 20 * time.Second
)

// siteLinksStatus is the JSON snapshot of the background catalog refresh job.
type siteLinksStatus struct {
	State      string     `json:"state"` // idle | running | done | error
	Total      int        `json:"total"`
	Crawled    int        `json:"crawled"`
	Products   int        `json:"products"`
	Articles   int        `json:"articles"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func (s *Server) siteLinksSnapshot() siteLinksStatus {
	s.siteLinksMu.Lock()
	defer s.siteLinksMu.Unlock()
	status := s.siteLinksJob
	if status.State == "" {
		status.State = "idle"
	}
	return status
}

func (s *Server) updateSiteLinksStatus(update func(*siteLinksStatus)) {
	s.siteLinksMu.Lock()
	defer s.siteLinksMu.Unlock()
	update(&s.siteLinksJob)
}

// tryStartSiteLinksRefresh claims the single job slot; false when a refresh is
// already running.
func (s *Server) tryStartSiteLinksRefresh() bool {
	s.siteLinksMu.Lock()
	defer s.siteLinksMu.Unlock()
	if s.siteLinksJob.State == "running" {
		return false
	}
	now := time.Now().UTC()
	s.siteLinksJob = siteLinksStatus{State: "running", StartedAt: &now}
	return true
}

// handleRefreshSiteLinks starts a background re-crawl of the shop sitemap. The
// crawl of a large catalog takes many minutes, so the request only kicks the
// job off; progress is polled via handleSiteLinksRefreshStatus. By default the
// crawl is incremental (only URLs missing from the persisted product-link
// list); ?full=1 re-crawls everything.
func (s *Server) handleRefreshSiteLinks(w http.ResponseWriter, r *http.Request) {
	sitemapURL := s.effectiveSitemapURL(r.Context())
	if sitemapURL == "" {
		writeError(w, http.StatusBadRequest, errors.New("укажите адрес магазина или sitemap в Настройках, чтобы обновлять каталог товаров"))
		return
	}
	full := r.URL.Query().Get("full") == "1" || r.URL.Query().Get("full") == "true"

	if !s.tryStartSiteLinksRefresh() {
		writeJSON(w, http.StatusConflict, s.siteLinksSnapshot())
		return
	}
	go s.runSiteLinksRefresh(sitemapURL, full)
	writeJSON(w, http.StatusAccepted, s.siteLinksSnapshot())
}

func (s *Server) handleSiteLinksRefreshStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.siteLinksSnapshot())
}

func (s *Server) runSiteLinksRefresh(sitemapURL string, full bool) {
	ctx, cancel := context.WithTimeout(context.Background(), siteLinksJobTimeout)
	defer cancel()
	client := &http.Client{Timeout: siteLinksFetchTimeout}

	fail := func(err error) {
		now := time.Now().UTC()
		s.updateSiteLinksStatus(func(status *siteLinksStatus) {
			status.State = "error"
			status.Error = err.Error()
			status.FinishedAt = &now
		})
		s.logger.Error("site links refresh failed", "error", err)
	}

	sitemapURLs, err := site.FetchProductURLs(ctx, client, sitemapURL)
	if err != nil {
		fail(fmt.Errorf("не удалось прочитать sitemap: %w", err))
		return
	}

	var known []site.ProductLink
	if !full {
		if known, err = s.productCatalogLinks(); err != nil {
			fail(err)
			return
		}
	}
	todo := site.NewProductURLs(sitemapURLs, known)
	s.updateSiteLinksStatus(func(status *siteLinksStatus) { status.Total = len(todo) })

	crawled, crawlErr := site.CrawlProductLinks(ctx, client, todo, func(done int) {
		s.updateSiteLinksStatus(func(status *siteLinksStatus) { status.Crawled = done })
	})
	if crawlErr != nil && len(crawled) == 0 && len(todo) > 0 {
		fail(fmt.Errorf("обход каталога не удался: %w", crawlErr))
		return
	}

	merged := site.MergeProductLinks(known, sitemapURLs, crawled)

	// The crawl may have consumed the whole job budget; persisting and
	// regenerating the export must still succeed, so they run on a fresh
	// context.
	exportCtx, cancelExport := context.WithTimeout(context.Background(), siteLinksExportTimeout)
	defer cancelExport()
	products, articles, err := s.regenerateSiteData(exportCtx, merged)
	if err != nil {
		fail(err)
		return
	}

	now := time.Now().UTC()
	s.updateSiteLinksStatus(func(status *siteLinksStatus) {
		status.Products = products
		status.Articles = articles
		status.FinishedAt = &now
		if crawlErr != nil {
			status.State = "error"
			status.Error = fmt.Sprintf("успели обойти %d из %d новых товаров — каталог сохранён частично, запустите обновление ещё раз, обход продолжится с этого места (%v)", status.Crawled, status.Total, crawlErr)
		} else {
			status.State = "done"
		}
	})
	s.logger.Info("site links refreshed", "products", products, "articles", articles, "crawled", len(crawled), "new_urls", len(todo))
}

// regenerateSiteData persists the crawled product links, swaps the in-memory
// article→URL map, and rewrites the static reviews-data export (bundles +
// links index). Separated from the crawl so it is testable without network.
func (s *Server) regenerateSiteData(ctx context.Context, links []site.ProductLink) (products int, articles int, err error) {
	if s.cfg.ProductLinksPath != "" {
		if err = os.MkdirAll(filepath.Dir(s.cfg.ProductLinksPath), 0o755); err != nil {
			return 0, 0, err
		}
		file, createErr := os.Create(s.cfg.ProductLinksPath)
		if createErr != nil {
			return 0, 0, createErr
		}
		if encErr := site.EncodeProductLinks(file, links); encErr != nil {
			file.Close()
			return 0, 0, encErr
		}
		if closeErr := file.Close(); closeErr != nil {
			return 0, 0, closeErr
		}
	}

	s.setProductLinks(site.ProductLinkMap(links))

	reviews, err := s.store.ListVisibleReviews(ctx)
	if err != nil {
		return 0, 0, err
	}
	pins, err := s.store.AllShowcasePins(ctx)
	if err != nil {
		return 0, 0, err
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.productLinks(),
		MarketplacePolicy:  s.activeMarketplacePolicy(ctx, "product"),
	}
	bundles := staticexport.BuildBundles(reviews, mapper, pins)

	generatedAt := time.Now().UTC()
	outDir := filepath.Join(s.cfg.StaticDir, "reviews-data")
	if err = staticexport.Write(outDir, bundles, generatedAt); err != nil {
		return 0, 0, err
	}
	if err = staticexport.WriteLinks(outDir, staticexport.BuildLinkIndex(links, generatedAt)); err != nil {
		return 0, 0, err
	}

	return len(links), len(bundles), nil
}
