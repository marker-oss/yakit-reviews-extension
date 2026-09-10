package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type articlePinsResponse struct {
	ReviewIDs []uint `json:"reviewIds"`
}

type replaceArticlePinsRequest struct {
	ReviewIDs []uint `json:"reviewIds"`
}

func (s *Server) handleListArticlePins(w http.ResponseWriter, r *http.Request) {
	article := strings.TrimSpace(r.PathValue("article"))
	if article == "" {
		writeError(w, http.StatusBadRequest, errors.New("article is required"))
		return
	}
	pins, err := s.store.ListShowcasePins(r.Context(), article)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ids := make([]uint, 0, len(pins))
	for _, pin := range pins {
		ids = append(ids, pin.ReviewID)
	}
	writeJSON(w, http.StatusOK, articlePinsResponse{ReviewIDs: ids})
}

func (s *Server) handleReplaceArticlePins(w http.ResponseWriter, r *http.Request) {
	article := strings.TrimSpace(r.PathValue("article"))
	if article == "" {
		writeError(w, http.StatusBadRequest, errors.New("article is required"))
		return
	}
	var req replaceArticlePinsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if err := s.store.ReplaceShowcasePins(r.Context(), article, uniqueReviewIDs(req.ReviewIDs)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveArticlePin(w http.ResponseWriter, r *http.Request) {
	article := strings.TrimSpace(r.PathValue("article"))
	if article == "" {
		writeError(w, http.StatusBadRequest, errors.New("article is required"))
		return
	}
	reviewID, err := strconv.ParseUint(r.PathValue("reviewID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	if err := s.store.RemoveShowcasePin(r.Context(), article, uint(reviewID)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func uniqueReviewIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
