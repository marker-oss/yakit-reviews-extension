package store

import (
	"context"

	"gorm.io/gorm"
)

// ListAllReviews returns every review with media preloaded, newest first.
// It is intended for full static export, not paginated API responses.
func (s *Store) ListAllReviews(ctx context.Context) ([]Review, error) {
	var reviews []Review
	if err := s.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).
		Order("created_at_mp desc").
		Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}
