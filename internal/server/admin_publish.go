package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	staticexport "reviews/internal/export"
	"reviews/internal/reviewjson"
	"reviews/internal/site"
)

type publishResult struct {
	GeneratedAt time.Time
	Articles    int
	Reviews     int
}

// publishReviewsData regenerates the static reviews-data export. Shared by
// the admin «Опубликовать» handler and the auto-publish loop.
func (s *Server) publishReviewsData(ctx context.Context) (publishResult, error) {
	reviews, err := s.store.ListVisibleReviews(ctx)
	if err != nil {
		return publishResult{}, err
	}
	pins, err := s.store.AllShowcasePins(ctx)
	if err != nil {
		return publishResult{}, err
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.productLinks(),
		MarketplacePolicy:  s.activeMarketplacePolicy(ctx, "product"),
	}
	bundles := staticexport.BuildBundles(reviews, mapper, pins)
	generatedAt := time.Now().UTC()
	outDir := filepath.Join(s.cfg.StaticDir, "reviews-data")
	if err := staticexport.Write(outDir, bundles, generatedAt); err != nil {
		return publishResult{}, err
	}
	if links, err := s.productCatalogLinks(); err != nil {
		return publishResult{}, err
	} else if len(links) > 0 {
		linkIndex := staticexport.BuildLinkIndex(links, generatedAt)
		if err := staticexport.WriteLinks(outDir, linkIndex); err != nil {
			return publishResult{}, err
		}
	}
	return publishResult{GeneratedAt: generatedAt, Articles: len(bundles), Reviews: len(reviews)}, nil
}

func (s *Server) handlePublishReviewsData(w http.ResponseWriter, r *http.Request) {
	result, err := s.publishReviewsData(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// A manual publish covers everything up to now.
	if err := s.store.MarkExportPublished(r.Context(), result.GeneratedAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"generatedAt": result.GeneratedAt,
		"articles":    result.Articles,
		"reviews":     result.Reviews,
	})
}

func (s *Server) productCatalogLinks() ([]site.ProductLink, error) {
	if s.cfg.ProductLinksPath == "" {
		return nil, nil
	}
	file, err := os.Open(s.cfg.ProductLinksPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return site.LoadProductLinks(file)
}
