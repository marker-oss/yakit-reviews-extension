package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	staticexport "reviews/internal/export"
	"reviews/internal/reviewjson"
	"reviews/internal/site"
)

// handleRefreshSiteLinks re-crawls the shop sitemap, rewrites the product-link
// file, and regenerates the static export so newly added products are picked up
// without using the CLI. Configured by REVIEWS_SITE_SITEMAP_URL.
func (s *Server) handleRefreshSiteLinks(w http.ResponseWriter, r *http.Request) {
	sitemapURL := s.effectiveSitemapURL(r.Context())
	if sitemapURL == "" {
		writeError(w, http.StatusBadRequest, errors.New("укажите адрес магазина или sitemap в Настройках, чтобы обновлять каталог товаров"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	links, err := site.DiscoverKitProductLinks(ctx, sitemapURL, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	products, articles, err := s.regenerateSiteData(ctx, links)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"products": products,
		"articles": articles,
	})
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
