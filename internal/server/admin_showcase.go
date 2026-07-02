package server

import (
	"context"
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
	marketplacePolicy := s.activeMarketplacePolicy(r.Context(), "homepage")
	aggregate, err := s.publicShowcaseAggregate(r.Context(), marketplacePolicy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.productLinks(),
		MarketplacePolicy:  marketplacePolicy,
	}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, rv := range reviews {
		if mapper.ReviewHidden(rv) {
			continue
		}
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

func publicReviewAggregate(reviews []reviewjson.Review) reviewAggregateResponse {
	var sum, ratingCount int64
	for _, review := range reviews {
		if review.Rating == nil {
			continue
		}
		sum += int64(*review.Rating)
		ratingCount++
	}
	aggregate := reviewAggregateResponse{
		TotalReviews: int64(len(reviews)),
		RatingCount:  ratingCount,
	}
	if ratingCount > 0 {
		aggregate.AverageRating = float64(sum) / float64(ratingCount)
	}
	return aggregate
}

func (s *Server) publicShowcaseAggregate(ctx context.Context, policy reviewjson.MarketplacePolicies) (reviewAggregateResponse, error) {
	if len(policy.ExcludedMarketplaces()) == 0 {
		aggregate, err := s.store.VisibleReviewAggregate(ctx)
		if err != nil {
			return reviewAggregateResponse{}, err
		}
		return reviewAggregateResponse{
			TotalReviews:  aggregate.TotalReviews,
			RatingCount:   aggregate.RatingCount,
			AverageRating: aggregate.AverageRating,
		}, nil
	}
	reviews, err := s.store.ListVisibleReviews(ctx)
	if err != nil {
		return reviewAggregateResponse{}, err
	}
	mapper := reviewjson.Mapper{MarketplacePolicy: policy}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, rv := range reviews {
		if mapper.ReviewHidden(rv) {
			continue
		}
		items = append(items, mapper.ToReview(rv))
	}
	return publicReviewAggregate(items), nil
}
