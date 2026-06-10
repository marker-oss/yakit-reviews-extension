package store

import "time"

// ShowcaseRule holds the auto-selection rules for the homepage showcase.
// Exactly one row per tenant is expected.
type ShowcaseRule struct {
	ID           uint   `gorm:"primaryKey"`
	TenantID     uint   `gorm:"not null;default:1;uniqueIndex"`
	MinRating    int    `gorm:"not null;default:4"`
	RequirePhoto bool   `gorm:"not null;default:false"`
	MinTextLen   int    `gorm:"not null;default:0"`
	MaxAgeDays   int    `gorm:"not null;default:0"`
	SortBy       string `gorm:"size:16;not null;default:recent"`
	Limit        int    `gorm:"not null;default:12"`
	UpdatedAt    time.Time
}
