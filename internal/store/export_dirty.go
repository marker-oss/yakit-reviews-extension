package store

import (
	"context"
	"time"
)

// app_settings keys backing the export staleness marker. DB-backed so the
// auto-publish loop picks up pending changes across restarts.
const (
	settingExportDirtyAt     = "export_dirty_at"
	settingExportPublishedAt = "export_published_at"
)

// MarkExportDirty records that exportable data changed since the last publish.
func (s *Store) MarkExportDirty(ctx context.Context) error {
	return s.SetAppSetting(ctx, settingExportDirtyAt, time.Now().UTC().Format(time.RFC3339Nano))
}

// markExportDirtyBestEffort flags the export as stale after a successful data
// mutation. Failing to set the flag must never fail the mutation itself; the
// worst case is one delayed republish.
func (s *Store) markExportDirtyBestEffort(ctx context.Context) {
	_ = s.MarkExportDirty(ctx)
}

// dirtyOnSuccess marks the export stale when a curation mutation succeeded and
// passes the mutation's error through unchanged.
func (s *Store) dirtyOnSuccess(ctx context.Context, err error) error {
	if err == nil {
		s.markExportDirtyBestEffort(ctx)
	}
	return err
}

// ExportDirtySince reports whether the static export is stale and the moment
// of the newest unexported change. Publishers must pass that moment back to
// MarkExportPublished so changes racing with an in-flight export stay dirty.
func (s *Store) ExportDirtySince(ctx context.Context) (time.Time, bool, error) {
	dirtyRaw, err := s.GetAppSetting(ctx, settingExportDirtyAt)
	if err != nil || dirtyRaw == "" {
		return time.Time{}, false, err
	}
	dirtyAt, err := time.Parse(time.RFC3339Nano, dirtyRaw)
	if err != nil {
		return time.Time{}, false, err
	}
	publishedRaw, err := s.GetAppSetting(ctx, settingExportPublishedAt)
	if err != nil {
		return time.Time{}, false, err
	}
	if publishedRaw != "" {
		if publishedAt, err := time.Parse(time.RFC3339Nano, publishedRaw); err == nil && !dirtyAt.After(publishedAt) {
			return time.Time{}, false, nil
		}
	}
	return dirtyAt, true, nil
}

// MarkExportPublished records a successful export of all changes up to upTo.
func (s *Store) MarkExportPublished(ctx context.Context, upTo time.Time) error {
	return s.SetAppSetting(ctx, settingExportPublishedAt, upTo.UTC().Format(time.RFC3339Nano))
}
