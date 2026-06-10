package store

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type ReviewListFilter struct {
	Marketplace   string
	Rating        int
	Limit         int
	Offset        int
	Visibility    string
	SellerArticle string
	HasPhoto      bool
	Search        string
	PinnedFirst   bool
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
		})

	query, err := s.applyReviewFilters(query, filter)
	if err != nil {
		return nil, err
	}
	if filter.PinnedFirst {
		query = query.Order("pinned desc")
	}
	query = query.Order("created_at_mp desc").Limit(limit).Offset(filter.Offset)

	var reviews []Review
	if err := query.Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *Store) applyReviewFilters(query *gorm.DB, filter ReviewListFilter) (*gorm.DB, error) {
	if filter.Marketplace != "" && filter.Marketplace != "all" {
		query = query.Where("marketplace = ?", strings.ToLower(filter.Marketplace))
	}
	if filter.Rating > 0 {
		if filter.Rating < 1 || filter.Rating > 5 {
			return nil, fmt.Errorf("rating must be between 1 and 5")
		}
		query = query.Where("rating = ?", filter.Rating)
	}
	if filter.Visibility != "" {
		query = query.Where("visibility = ?", filter.Visibility)
	}
	if filter.SellerArticle != "" {
		query = query.Where("seller_article = ?", filter.SellerArticle)
	}
	if filter.HasPhoto {
		query = query.Where("id IN (?)",
			s.db.Model(&ReviewMedia{}).Select("review_id").Where("kind = ?", "photo"))
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where(
			"text LIKE ? OR author_name LIKE ? OR pros LIKE ? OR cons LIKE ?",
			like, like, like, like,
		)
	}
	return query, nil
}
