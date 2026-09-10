package server

import (
	"net/http"

	"reviews/internal/store"
)

func (s *Server) handleCounts(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	pendingQ, err := s.store.ListQuestions(r.Context(), storeQuestionPendingFilter())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pendingReviews":   stats.PendingReviews,
		"pendingQuestions": len(pendingQ),
	})
}

func storeQuestionPendingFilter() store.QuestionFilter {
	return store.QuestionFilter{Status: "pending", Limit: 1000}
}
