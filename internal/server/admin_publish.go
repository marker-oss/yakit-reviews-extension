package server

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	staticexport "reviews/internal/export"
	"reviews/internal/reviewjson"
	"reviews/internal/site"
)

func (s *Server) handlePublishReviewsData(w http.ResponseWriter, r *http.Request) {
	reviews, err := s.store.ListVisibleReviews(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	pins, err := s.store.AllShowcasePins(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.productLinks(),
	}
	bundles := staticexport.BuildBundles(reviews, mapper, pins)
	generatedAt := time.Now().UTC()
	outDir := filepath.Join(s.cfg.StaticDir, "reviews-data")
	if err := staticexport.Write(outDir, bundles, generatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if links, err := s.productCatalogLinks(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	} else if len(links) > 0 {
		linkIndex := staticexport.BuildLinkIndex(links, generatedAt)
		if err := staticexport.WriteLinks(outDir, linkIndex); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"generatedAt": generatedAt,
		"articles":    len(bundles),
		"reviews":     len(reviews),
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
