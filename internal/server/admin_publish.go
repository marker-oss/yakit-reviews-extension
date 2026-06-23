package server

import (
	"net/http"
	"path/filepath"
	"time"

	staticexport "reviews/internal/export"
	"reviews/internal/reviewjson"
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
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"generatedAt": generatedAt,
		"articles":    len(bundles),
		"reviews":     len(reviews),
	})
}
