package store

import "context"

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
	TotalReviews  int64
	AverageRating float64
	ByMarketplace map[string]int64
}

func (s *Store) DashboardStats(ctx context.Context) (Stats, error) {
	stats := Stats{ByMarketplace: map[string]int64{}}
	db := s.db.WithContext(ctx)

	if err := db.Model(&Review{}).Count(&stats.TotalReviews).Error; err != nil {
		return Stats{}, err
	}

	var avg *float64
	if err := db.Model(&Review{}).Select("AVG(rating)").Scan(&avg).Error; err != nil {
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
