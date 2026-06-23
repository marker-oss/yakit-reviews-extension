package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type adminReview struct {
	reviewjson.Review
	Visibility string      `json:"visibility"`
	Pinned     bool        `json:"pinned"`
	Status     string      `json:"status"`
	AdminReply *adminReply `json:"adminReply,omitempty"`
}

type adminReply struct {
	Text string     `json:"text"`
	At   *time.Time `json:"at,omitempty"`
}

type adminReviewsResponse struct {
	Reviews []adminReview `json:"reviews"`
	Total   int64         `json:"total"`
}

func (s *Server) handleAdminReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, ok := adminStatus(q.Get("status"))
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("status must be active, deleted, or all"))
		return
	}
	filter := store.ReviewListFilter{
		Marketplace:   q.Get("marketplace"),
		Status:        status,
		Visibility:    q.Get("visibility"),
		SellerArticle: q.Get("article_exact"),
		ArticleSearch: firstNonEmpty(q.Get("article_search"), q.Get("article")),
		Search:        q.Get("search"),
		HasPhoto:      q.Get("has_photo") == "true",
		SortBy:        adminSortBy(q.Get("sort")),
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

	mapper := reviewjson.Mapper{ProductURLTemplate: s.cfg.ProductURLTemplate, ProductLinks: s.productLinks()}
	items := make([]adminReview, 0, len(reviews))
	for _, rv := range reviews {
		item := adminReview{
			Review:     mapper.ToReview(rv),
			Visibility: rv.Visibility,
			Pinned:     rv.Pinned,
			Status:     rv.Status,
		}
		if rv.AdminReplyText != nil && *rv.AdminReplyText != "" {
			item.AdminReply = &adminReply{Text: *rv.AdminReplyText, At: rv.AdminReplyAt}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, adminReviewsResponse{Reviews: items, Total: total})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func adminStatus(value string) (string, bool) {
	switch value {
	case "", "active":
		return "", true
	case "deleted", "all":
		return value, true
	default:
		return "", false
	}
}

// adminSortBy whitelists the sort values exposed in the admin reviews UI.
// Anything else (including "newest") falls back to the store default: pinned
// first, then newest by marketplace creation date.
func adminSortBy(value string) string {
	switch value {
	case "highest", "lowest", "media":
		return value
	default:
		return ""
	}
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

func (s *Server) handleAdminReviewDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	if err := s.store.SoftDeleteReview(r.Context(), uint(id)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminReviewRestore(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	if err := s.store.RestoreReview(r.Context(), uint(id)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type replyRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleAdminReviewReply(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid review id"))
		return
	}
	var req replyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if err := s.store.SetReviewReply(r.Context(), uint(id), &req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type bulkModerationRequest struct {
	IDs        []uint  `json:"ids"`
	Visibility *string `json:"visibility"`
	Pinned     *bool   `json:"pinned"`
	Status     *string `json:"status"`
}

func (s *Server) handleAdminReviewsBulkModerate(w http.ResponseWriter, r *http.Request) {
	var req bulkModerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("ids must not be empty"))
		return
	}
	if req.Visibility == nil && req.Pinned == nil && req.Status == nil {
		writeError(w, http.StatusBadRequest, errors.New("nothing to update"))
		return
	}
	if req.Visibility != nil {
		if *req.Visibility != "visible" && *req.Visibility != "hidden" {
			writeError(w, http.StatusBadRequest, errors.New("visibility must be visible or hidden"))
			return
		}
		if err := s.store.SetReviewsVisibility(r.Context(), req.IDs, *req.Visibility); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Pinned != nil {
		if err := s.store.SetReviewsPinned(r.Context(), req.IDs, *req.Pinned); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Status != nil {
		if *req.Status != "deleted" {
			writeError(w, http.StatusBadRequest, errors.New("status must be deleted"))
			return
		}
		if err := s.store.SetReviewsStatus(r.Context(), req.IDs, *req.Status); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "updated": len(req.IDs)})
}
