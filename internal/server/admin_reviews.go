package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type adminReview struct {
	reviewjson.Review
	Visibility string `json:"visibility"`
	Pinned     bool   `json:"pinned"`
}

type adminReviewsResponse struct {
	Reviews []adminReview `json:"reviews"`
	Total   int64         `json:"total"`
}

func (s *Server) handleAdminReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.ReviewListFilter{
		Marketplace:   q.Get("marketplace"),
		Visibility:    q.Get("visibility"),
		SellerArticle: q.Get("article"),
		Search:        q.Get("search"),
		HasPhoto:      q.Get("has_photo") == "true",
		Limit:         parseInt(q.Get("limit"), 50),
		Offset:        parseInt(q.Get("offset"), 0),
		PinnedFirst:   true,
	}
	if rating := q.Get("rating"); rating != "" && rating != "all" {
		parsed, err := strconv.Atoi(rating)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("rating must be a number"))
			return
		}
		filter.Rating = parsed
	}

	reviews, total, err := s.store.ListReviewsWithCount(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	mapper := reviewjson.Mapper{ProductURLTemplate: s.cfg.ProductURLTemplate, ProductLinks: s.cfg.ProductLinks}
	items := make([]adminReview, 0, len(reviews))
	for _, rv := range reviews {
		items = append(items, adminReview{
			Review:     mapper.ToReview(rv),
			Visibility: rv.Visibility,
			Pinned:     rv.Pinned,
		})
	}
	writeJSON(w, http.StatusOK, adminReviewsResponse{Reviews: items, Total: total})
}

type moderationRequest struct {
	Visibility *string `json:"visibility"`
	Pinned     *bool   `json:"pinned"`
}

func (s *Server) handleAdminReviewModerate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}

	var req moderationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if req.Visibility != nil {
		if *req.Visibility != "visible" && *req.Visibility != "hidden" {
			writeError(w, http.StatusBadRequest, errors.New("visibility must be visible or hidden"))
			return
		}
		if err := s.store.SetReviewVisibility(r.Context(), uint(id), *req.Visibility); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Pinned != nil {
		if err := s.store.SetReviewPinned(r.Context(), uint(id), *req.Pinned); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
