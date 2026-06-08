package store

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) GetSyncState(ctx context.Context, marketplaceID string) (SyncState, error) {
	var state SyncState
	err := s.db.WithContext(ctx).First(&state, "marketplace = ?", marketplaceID).Error
	if err == nil {
		return state, nil
	}
	if err == gorm.ErrRecordNotFound {
		return SyncState{Marketplace: marketplaceID}, nil
	}
	return SyncState{}, err
}

func (s *Store) SaveSyncState(ctx context.Context, state SyncState) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "marketplace"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_synced_at", "backfilled"}),
	}).Create(&state).Error
}

func (s *Store) CreateSyncRun(ctx context.Context, marketplaceID string, startedAt time.Time) (SyncRun, error) {
	run := SyncRun{
		Marketplace: marketplaceID,
		StartedAt:   startedAt,
		Status:      "running",
	}
	err := s.db.WithContext(ctx).Create(&run).Error
	return run, err
}

func (s *Store) FinishSyncRun(ctx context.Context, id uint, status string, seen int, upserted int, errText *string) error {
	finishedAt := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&SyncRun{}).Where("id = ?", id).Updates(map[string]any{
		"finished_at":      &finishedAt,
		"status":           status,
		"reviews_seen":     seen,
		"reviews_upserted": upserted,
		"error_text":       errText,
	}).Error
}
