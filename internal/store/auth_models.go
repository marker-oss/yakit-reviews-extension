package store

import "time"

type AdminUser struct {
	ID           uint      `gorm:"primaryKey"`
	TenantID     uint      `gorm:"not null;default:1;index"`
	Login        string    `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string    `gorm:"not null"`
	// Role separates the SaaS operator from tenant admins: "owner" bypasses
	// the tenant scope for operator routes (used by the closed-source
	// overlay); "admin" is the normal tenant-scoped user. Default keeps
	// existing rows valid; the open-source binary never reads it.
	Role      string    `gorm:"size:16;not null;default:'admin'"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Session struct {
	Token     string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"not null;index"`
	User      AdminUser `gorm:"constraint:OnDelete:CASCADE"`
	TenantID  uint      `gorm:"not null;default:1;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}
