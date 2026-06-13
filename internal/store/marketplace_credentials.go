package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type MarketplaceCredential struct {
	ID          uint   `gorm:"primaryKey"`
	TenantID    uint   `gorm:"not null;default:1;uniqueIndex:idx_marketplace_credentials_tenant_marketplace"`
	Marketplace string `gorm:"size:32;not null;uniqueIndex:idx_marketplace_credentials_tenant_marketplace"`
	Enabled     bool   `gorm:"not null;default:true"`
	Payload     string `gorm:"type:text;not null;default:'{}'"`
	UpdatedAt   time.Time
}

type MarketplaceCredentialPatch struct {
	Marketplace string
	Enabled     *bool
	Values      map[string]string
}

func (s *Store) GetMarketplaceCredential(ctx context.Context, marketplaceID string) (MarketplaceCredential, error) {
	var cred MarketplaceCredential
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace = ?", DefaultTenantID, marketplaceID).
		First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MarketplaceCredential{}, ErrNotFound
	}
	return cred, err
}

func (s *Store) ListMarketplaceCredentials(ctx context.Context) ([]MarketplaceCredential, error) {
	var creds []MarketplaceCredential
	err := s.db.WithContext(ctx).
		Where("tenant_id = ?", DefaultTenantID).
		Order("marketplace asc").
		Find(&creds).Error
	return creds, err
}

func (s *Store) SaveMarketplaceCredential(ctx context.Context, patch MarketplaceCredentialPatch) (MarketplaceCredential, error) {
	var saved MarketplaceCredential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_id = ? AND marketplace = ?", DefaultTenantID, patch.Marketplace).
			First(&saved).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			saved = MarketplaceCredential{
				TenantID:    DefaultTenantID,
				Marketplace: patch.Marketplace,
				Enabled:     true,
				Payload:     "{}",
			}
		} else if err != nil {
			return err
		}

		payload := map[string]string{}
		if saved.Payload != "" {
			if err := json.Unmarshal([]byte(saved.Payload), &payload); err != nil {
				return err
			}
		}
		for key, value := range patch.Values {
			if value == "" {
				continue
			}
			payload[key] = value
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if patch.Enabled != nil {
			saved.Enabled = *patch.Enabled
		}
		saved.Payload = string(body)

		if saved.ID == 0 {
			return tx.Create(&saved).Error
		}
		return tx.Save(&saved).Error
	})
	return saved, err
}

func (c MarketplaceCredential) PayloadMap() map[string]string {
	values := map[string]string{}
	if c.Payload == "" {
		return values
	}
	_ = json.Unmarshal([]byte(c.Payload), &values)
	return values
}
