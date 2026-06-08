package store

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type ReviewListFilter struct {
	Marketplace string
	Rating      int
	Limit       int
	Offset      int
}

func (s *Store) ListReviews(ctx context.Context, filter ReviewListFilter) ([]Review, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := s.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).
		Order("created_at_mp desc").
		Limit(limit).
		Offset(filter.Offset)

	if filter.Marketplace != "" && filter.Marketplace != "all" {
		query = query.Where("marketplace = ?", strings.ToLower(filter.Marketplace))
	}
	if filter.Rating > 0 {
		if filter.Rating < 1 || filter.Rating > 5 {
			return nil, fmt.Errorf("rating must be between 1 and 5")
		}
		query = query.Where("rating = ?", filter.Rating)
	}

	var reviews []Review
	if err := query.Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}
