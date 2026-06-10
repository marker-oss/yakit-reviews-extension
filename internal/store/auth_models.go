package store

import "time"

// DefaultTenantID is the single implicit tenant used until multi-tenancy is
// enabled. All tenant-scoped rows use this value for now.
const DefaultTenantID uint = 1

type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	TenantID     uint   `gorm:"not null;default:1;index"`
	Login        string `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	Token     string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"not null;index"`
	User      AdminUser `gorm:"constraint:OnDelete:CASCADE"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}
