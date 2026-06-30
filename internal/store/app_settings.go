package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Setting keys persisted in the app_settings table.
const (
	// SettingAgreementURL is the URL of the seller's user-agreement / personal
	// data consent page shown next to the public submission form's consent
	// checkbox. Editable from the admin panel; falls back to REVIEWS_PRIVACY_URL.
	SettingAgreementURL = "agreement_url"
	// SettingShopOrigin is the seller's shop origin (e.g. https://shop.ru). When
	// no explicit sitemap URL is set, the catalog refresh derives the sitemap as
	// <shop_origin>/sitemap.xml. Editable from the admin panel.
	SettingShopOrigin = "shop_origin"
	// SettingSitemapURL is an explicit shop sitemap URL crawled by the catalog
	// refresh. Takes priority over SettingShopOrigin; falls back to the env
	// REVIEWS_SITE_SITEMAP_URL. Editable from the admin panel.
	SettingSitemapURL = "sitemap_url"
	// SettingPublishRepliesPrefix + marketplace id is the per-marketplace
	// "publish seller replies back to the marketplace" toggle ("true"/"").
	SettingPublishRepliesPrefix = "publish_replies_"
)

// PublishRepliesKey is the app_settings key for a marketplace's publish toggle.
func PublishRepliesKey(marketplace string) string {
	return SettingPublishRepliesPrefix + marketplace
}

// AppSetting is a tenant-scoped key/value record for admin-editable settings
// that need to outlive the process (unlike env-only config).
type AppSetting struct {
	ID        uint   `gorm:"primaryKey"`
	TenantID  uint   `gorm:"not null;default:1;uniqueIndex:idx_app_settings_tenant_key"`
	Key       string `gorm:"size:64;not null;uniqueIndex:idx_app_settings_tenant_key"`
	Value     string `gorm:"type:text;not null;default:''"`
	UpdatedAt time.Time
}

// GetAppSetting returns the stored value for key, or "" if it has never been
// set. A missing row is not an error.
func (s *Store) GetAppSetting(ctx context.Context, key string) (string, error) {
	var setting AppSetting
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND key = ?", DefaultTenantID, key).
		First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// SetAppSetting upserts the value for key. The value is trimmed.
func (s *Store) SetAppSetting(ctx context.Context, key, value string) error {
	setting := AppSetting{
		TenantID: DefaultTenantID,
		Key:      key,
		Value:    strings.TrimSpace(value),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&setting).Error
}
