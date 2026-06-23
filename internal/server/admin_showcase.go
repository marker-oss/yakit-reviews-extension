package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

func (s *Server) handleGetShowcaseRule(w http.ResponseWriter, r *http.Request) {
	rule, err := s.store.GetShowcaseRule(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handlePutShowcaseRule(w http.ResponseWriter, r *http.Request) {
	var rule store.ShowcaseRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if err := s.store.SaveShowcaseRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleShowcase is the public endpoint used by the homepage widget.
func (s *Server) handleShowcase(w http.ResponseWriter, r *http.Request) {
	rule, err := s.store.GetShowcaseRule(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reviews, err := s.store.ShowcaseReviews(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	aggregate, err := s.store.VisibleReviewAggregate(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{ProductURLTemplate: s.cfg.ProductURLTemplate, ProductLinks: s.productLinks()}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, rv := range reviews {
		items = append(items, mapper.ToReview(rv))
	}
	writeJSON(w, http.StatusOK, showcaseResponse{
		Reviews: items,
		Count:   len(items),
		Aggregate: reviewAggregateResponse{
			TotalReviews:  aggregate.TotalReviews,
			RatingCount:   aggregate.RatingCount,
			AverageRating: aggregate.AverageRating,
		},
	})
}

type showcaseResponse struct {
	Reviews   []reviewjson.Review     `json:"reviews"`
	Count     int                     `json:"count"`
	Aggregate reviewAggregateResponse `json:"aggregate"`
}

type reviewAggregateResponse struct {
	TotalReviews  int64   `json:"totalReviews"`
	RatingCount   int64   `json:"ratingCount"`
	AverageRating float64 `json:"averageRating"`
}
