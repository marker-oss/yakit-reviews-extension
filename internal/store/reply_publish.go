package store

import (
	"context"
	"time"
)

// SetReplyPublishState records the outcome of a publish attempt. errText is
// stored only for the "failed" state; publishedAt only for "published".
func (s *Store) SetReplyPublishState(ctx context.Context, id uint, state string, errText *string, publishedAt *time.Time) error {
	updates := map[string]any{
		"reply_publish_state": state,
		"reply_publish_error": errText,
		"reply_published_at":  publishedAt,
	}
	return s.db.WithContext(ctx).Model(&Review{}).Where("id = ?", id).Updates(updates).Error
}

// ReviewsNeedingReplyPublish returns marketplace reviews that carry a seller
// reply but have not been successfully published yet (pending, failed, or
// never attempted).
func (s *Store) ReviewsNeedingReplyPublish(ctx context.Context) ([]Review, error) {
	var rows []Review
	err := s.db.WithContext(ctx).
		Where("marketplace <> ?", MarketplaceSite).
		Where("admin_reply_text IS NOT NULL AND admin_reply_text <> ''").
		Where("reply_publish_state IS NULL OR reply_publish_state IN ?", []string{"pending", "failed"}).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}

func (s *Store) ReviewByID(ctx context.Context, id uint) (Review, error) {
	var review Review
	err := s.db.WithContext(ctx).First(&review, id).Error
	return review, err
}
