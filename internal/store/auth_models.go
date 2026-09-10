package store

import "time"

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
	TenantID  uint      `gorm:"not null;default:1;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}
