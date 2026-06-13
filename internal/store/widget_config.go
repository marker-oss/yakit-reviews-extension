package store

import (
	"context"

	"gorm.io/gorm"
)

func (s *Store) GetActiveWidgetConfig(ctx context.Context, widgetContext string) (WidgetConfig, error) {
	var cfg WidgetConfig
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND context = ? AND active = ?", DefaultTenantID, widgetContext, true).
		First(&cfg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return WidgetConfig{}, ErrNotFound
		}
		return WidgetConfig{}, err
	}
	return cfg, nil
}

func (s *Store) PublishWidgetConfig(ctx context.Context, widgetContext, payload string) (WidgetConfig, error) {
	var created WidgetConfig
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Update("active", false).Error; err != nil {
			return err
		}
		created = WidgetConfig{
			TenantID: DefaultTenantID,
			Context:  widgetContext,
			Version:  maxVersion + 1,
			Payload:  payload,
			Active:   true,
		}
		return tx.Create(&created).Error
	})
	return created, err
}

func (s *Store) ListWidgetConfigVersions(ctx context.Context, widgetContext string) ([]WidgetConfig, error) {
	var versions []WidgetConfig
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
		Order("version desc").
		Find(&versions).Error
	return versions, err
}

func (s *Store) SetActiveWidgetConfigVersion(ctx context.Context, widgetContext string, version int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cfg WidgetConfig
		if err := tx.Where("tenant_id = ? AND context = ? AND version = ?", DefaultTenantID, widgetContext, version).
			First(&cfg).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Model(&WidgetConfig{}).
			Where("tenant_id = ? AND context = ?", DefaultTenantID, widgetContext).
			Update("active", false).Error; err != nil {
			return err
		}
		return tx.Model(&WidgetConfig{}).Where("id = ?", cfg.ID).Update("active", true).Error
	})
}
