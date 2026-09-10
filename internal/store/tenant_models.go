package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Tenant is one seller shop on a shared instance (SaaS) — or the single
// implicit tenant (ID 1) in open-source single-tenant mode.
type Tenant struct {
	ID          uint      `gorm:"primaryKey"`
	Slug        string    `gorm:"size:64;not null;uniqueIndex"`
	PublicKey   string    `gorm:"size:64;not null;uniqueIndex"` // 32 bytes hex
	ShopOrigin  string    `gorm:"size:255;not null"`
	Plan        string    `gorm:"size:16;not null;default:'trial'"` // trial|free|base|pro|pro+
	Status      string    `gorm:"size:16;not null;default:'trial'"` // trial|active|grace|paused
	TrialEndsAt time.Time `gorm:"not null"`
	CreatedAt   time.Time
}

// EnsureDefaultTenant seeds the implicit tenant 1 once. PublicKey is
// generated per installation; slug and origin stay fixed for the single
// open-source tenant.
func (s *Store) EnsureDefaultTenant(ctx context.Context, shopOrigin string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&Tenant{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&Tenant{
		ID:         DefaultTenantID,
		Slug:       "default",
		PublicKey:  hex.EncodeToString(key),
		ShopOrigin: shopOrigin,
	}).Error
}

// TenantByID loads a tenant by primary key (resolved from the context).
func (s *Store) TenantByID(ctx context.Context) (Tenant, error) {
	var tenant Tenant
	err := s.db.WithContext(ctx).First(&tenant, TenantIDFromCtx(ctx)).Error
	return tenant, err
}

// TenantByPublicKey resolves a tenant by its public key. Returns ErrNotFound
// for an unknown key.
func (s *Store) TenantByPublicKey(ctx context.Context, publicKey string) (Tenant, error) {
	var tenant Tenant
	err := s.db.WithContext(ctx).Where("public_key = ?", publicKey).First(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Tenant{}, ErrNotFound
	}
	return tenant, err
}

// CreateTenant registers a new tenant with a generated 32-byte hex public key
// and a 14-day trial window.
func (s *Store) CreateTenant(ctx context.Context, slug, shopOrigin string) (Tenant, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Tenant{}, err
	}
	tenant := Tenant{
		Slug:        slug,
		PublicKey:   hex.EncodeToString(key),
		ShopOrigin:  shopOrigin,
		TrialEndsAt: time.Now().UTC().Add(14 * 24 * time.Hour),
	}
	err := s.db.WithContext(ctx).Create(&tenant).Error
	return tenant, err
}
