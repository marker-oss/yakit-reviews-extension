package server

import (
	"errors"
	"net/http"
)

var errSyncDisabled = errors.New("sync is disabled")

type MarketplaceStatus struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	runs, err := s.store.RecentSyncRuns(r.Context(), 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_reviews":  stats.TotalReviews,
		"average_rating": stats.AverageRating,
		"by_marketplace": stats.ByMarketplace,
		"recent_syncs":   runs,
	})
}

func (s *Server) handleMarketplaces(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"marketplaces": s.cfg.Marketplaces})
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TriggerSync == nil {
		writeError(w, http.StatusServiceUnavailable, errSyncDisabled)
		return
	}
	mp := r.URL.Query().Get("marketplace")
	var marketplaces []string
	if mp != "" {
		marketplaces = []string{mp}
	}
	go s.cfg.TriggerSync(marketplaces)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
