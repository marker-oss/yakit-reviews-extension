package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (s *Store) SetReviewVisibility(ctx context.Context, id uint, visibility string) error {
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id = ?", id).
		Update("visibility", visibility).Error
}

func (s *Store) SetReviewPinned(ctx context.Context, id uint, pinned bool) error {
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id = ?", id).
		Update("pinned", pinned).Error
}

// SetReviewsVisibility updates visibility for every id in one statement. An
// empty id set is a no-op.
func (s *Store) SetReviewsVisibility(ctx context.Context, ids []uint, visibility string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id IN ?", ids).
		Update("visibility", visibility).Error
}

// SetReviewsPinned updates the pinned flag for every id in one statement. An
// empty id set is a no-op.
func (s *Store) SetReviewsPinned(ctx context.Context, ids []uint, pinned bool) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id IN ?", ids).
		Update("pinned", pinned).Error
}

// ListReviewsWithCount returns a page of reviews plus the total matching count
// without limit/offset for pagination.
func (s *Store) ListReviewsWithCount(ctx context.Context, filter ReviewListFilter) ([]Review, int64, error) {
	items, err := s.ListReviews(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	query := s.db.WithContext(ctx).Model(&Review{})
	query, err = s.applyReviewFilters(query, filter)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type Stats struct {
	TotalReviews   int64
	VisibleReviews int64
	AverageRating  float64
	ByMarketplace  map[string]int64
}

type ReviewAggregate struct {
	TotalReviews  int64
	RatingCount   int64
	AverageRating float64
}

func (s *Store) VisibleReviewAggregate(ctx context.Context) (ReviewAggregate, error) {
	var aggregate ReviewAggregate
	db := s.db.WithContext(ctx).Model(&Review{}).Where("visibility = ?", "visible")

	if err := db.Count(&aggregate.TotalReviews).Error; err != nil {
		return ReviewAggregate{}, err
	}

	var ratings struct {
		RatingCount   int64
		AverageRating *float64
	}
	if err := db.
		Select("COUNT(rating) AS rating_count, AVG(rating) AS average_rating").
		Scan(&ratings).Error; err != nil {
		return ReviewAggregate{}, err
	}
	aggregate.RatingCount = ratings.RatingCount
	if ratings.AverageRating != nil {
		aggregate.AverageRating = *ratings.AverageRating
	}
	return aggregate, nil
}

func (s *Store) DashboardStats(ctx context.Context) (Stats, error) {
	stats := Stats{ByMarketplace: map[string]int64{}}
	db := s.db.WithContext(ctx)

	if err := db.Model(&Review{}).Count(&stats.TotalReviews).Error; err != nil {
		return Stats{}, err
	}

	visible := db.Model(&Review{}).Where("visibility = ?", "visible")
	if err := visible.Count(&stats.VisibleReviews).Error; err != nil {
		return Stats{}, err
	}

	// Average is computed over the visible set so the dashboard headline matches
	// what the public site shows via VisibleReviewAggregate / /api/showcase.
	var avg *float64
	if err := db.Model(&Review{}).Where("visibility = ?", "visible").
		Select("AVG(rating)").Scan(&avg).Error; err != nil {
		return Stats{}, err
	}
	if avg != nil {
		stats.AverageRating = *avg
	}

	var rows []struct {
		Marketplace string
		N           int64
	}
	if err := db.Model(&Review{}).
		Select("marketplace, COUNT(*) as n").
		Group("marketplace").
		Scan(&rows).Error; err != nil {
		return Stats{}, err
	}
	for _, row := range rows {
		stats.ByMarketplace[row.Marketplace] = row.N
	}
	return stats, nil
}

func (s *Store) RecentSyncRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	var runs []SyncRun
	err := s.db.WithContext(ctx).
		Order("started_at desc").
		Limit(limit).
		Find(&runs).Error
	return runs, err
}

func (s *Store) GetShowcaseRule(ctx context.Context) (ShowcaseRule, error) {
	var rule ShowcaseRule
	err := s.db.WithContext(ctx).
		Where("tenant_id = ?", DefaultTenantID).
		First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultShowcaseRule(), nil
	}
	return rule, err
}

func (s *Store) SaveShowcaseRule(ctx context.Context, rule ShowcaseRule) error {
	rule.TenantID = DefaultTenantID
	if rule.SortBy == "" {
		rule.SortBy = "recent"
	}
	if rule.Limit <= 0 {
		rule.Limit = 12
	}
	return s.db.WithContext(ctx).
		Where("tenant_id = ?", DefaultTenantID).
		Assign(rule).
		FirstOrCreate(&rule).Error
}

func (s *Store) ShowcaseReviews(ctx context.Context, rule ShowcaseRule) ([]Review, error) {
	query := s.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).
		Where("visibility = ?", "visible")
	if rule.MinRating > 0 {
		query = query.Where("rating >= ?", rule.MinRating)
	}
	if rule.RequirePhoto {
		query = query.Where("id IN (?)",
			s.db.Model(&ReviewMedia{}).Select("review_id").Where("kind = ?", "photo"))
	}
	if rule.MinTextLen > 0 {
		query = query.Where("LENGTH(text) >= ?", rule.MinTextLen)
	}
	if rule.MaxAgeDays > 0 {
		cutoff := timeNowUTC().AddDate(0, 0, -rule.MaxAgeDays)
		query = query.Where("created_at_mp >= ?", cutoff)
	}

	query = query.Order("pinned desc")
	if rule.SortBy == "rating" {
		query = query.Order("rating desc")
	}
	query = query.Order("created_at_mp desc")

	limit := rule.Limit
	if limit <= 0 || limit > 100 {
		limit = 12
	}

	var reviews []Review
	err := query.Limit(limit).Find(&reviews).Error
	return reviews, err
}

func defaultShowcaseRule() ShowcaseRule {
	return ShowcaseRule{
		TenantID:  DefaultTenantID,
		MinRating: 4,
		SortBy:    "recent",
		Limit:     12,
	}
}

var timeNowUTC = func() time.Time { return time.Now().UTC() }
