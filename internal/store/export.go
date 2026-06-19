package store

import (
	"context"

	"gorm.io/gorm"
)

// ListVisibleReviews returns every visible review with media preloaded, newest
// first. Hidden reviews are excluded so admin moderation ("hide") actually keeps
// them out of the static per-article JSON the product-page widget loads. It is
// intended for full static export, not paginated API responses.
func (s *Store) ListVisibleReviews(ctx context.Context) ([]Review, error) {
	var reviews []Review
	if err := s.db.WithContext(ctx).
		Where("visibility = ?", "visible").
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).
		Order("created_at_mp desc").
		Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}
