package store

import "time"

// WidgetConfig stores a versioned widget configuration payload for one render
// context, such as product cards or the homepage showcase.
type WidgetConfig struct {
	ID        uint   `gorm:"primaryKey"`
	TenantID  uint   `gorm:"not null;default:1;uniqueIndex:idx_widget_config_version;index:idx_widget_config_active"`
	Context   string `gorm:"size:32;not null;uniqueIndex:idx_widget_config_version;index:idx_widget_config_active"`
	Version   int    `gorm:"not null;uniqueIndex:idx_widget_config_version"`
	Payload   string `gorm:"type:text;not null"`
	Active    bool   `gorm:"not null;default:false;index:idx_widget_config_active"`
	CreatedAt time.Time
}
