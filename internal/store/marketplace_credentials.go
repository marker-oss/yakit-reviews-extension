package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"reviews/internal/secrets"

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

// GetMarketplaceCredential returns the tenant's credential for one
// marketplace with the payload decrypted (when a credentials key is set).
func (s *Store) GetMarketplaceCredential(ctx context.Context, marketplaceID string) (MarketplaceCredential, error) {
	var cred MarketplaceCredential
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace = ?", TenantIDFromCtx(ctx), marketplaceID).
		First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MarketplaceCredential{}, ErrNotFound
	}
	if err == nil {
		cred.Payload = s.decryptPayload(cred.Payload)
	}
	return cred, err
}

// ListMarketplaceCredentials returns every credential of the tenant with
// payloads decrypted (when a credentials key is set).
func (s *Store) ListMarketplaceCredentials(ctx context.Context) ([]MarketplaceCredential, error) {
	var creds []MarketplaceCredential
	err := s.db.WithContext(ctx).
		Where("tenant_id = ?", TenantIDFromCtx(ctx)).
		Order("marketplace asc").
		Find(&creds).Error
	if err == nil {
		for i := range creds {
			creds[i].Payload = s.decryptPayload(creds[i].Payload)
		}
	}
	return creds, err
}

func (s *Store) SaveMarketplaceCredential(ctx context.Context, patch MarketplaceCredentialPatch) (MarketplaceCredential, error) {
	var saved MarketplaceCredential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_id = ? AND marketplace = ?", TenantIDFromCtx(ctx), patch.Marketplace).
			First(&saved).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			saved = MarketplaceCredential{
				TenantID:    TenantIDFromCtx(ctx),
				Marketplace: patch.Marketplace,
				Enabled:     true,
				Payload:     "{}",
			}
		} else if err != nil {
			return err
		}
		// The stored row may be sealed; decrypt before merging (no-op for
		// plaintext legacy rows or when no key is configured).
		saved.Payload = s.decryptPayload(saved.Payload)
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
		if sealed, serr := s.encryptPayload(string(body)); serr != nil {
			return serr
		} else {
			saved.Payload = sealed
		}
		if saved.ID == 0 {
			return tx.Create(&saved).Error
		}
		return tx.Save(&saved).Error
	})
	if err == nil {
		saved.Payload = s.decryptPayload(saved.Payload)
	}
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

// SetCredentialsCipher installs the at-rest cipher for marketplace
// credentials. Call once after Open, before any request. Passing nil
// disables sealing (single-tenant compat).
func (s *Store) SetCredentialsCipher(c *secrets.Cipher) {
	s.credentials = c
}

// encryptPayload seals plaintext when a cipher is configured; "" and nil
// cipher pass through unchanged.
func (s *Store) encryptPayload(plaintext string) (string, error) {
	if s.credentials == nil || plaintext == "" {
		return plaintext, nil
	}
	return s.credentials.Encrypt(plaintext)
}

// decryptPayload opens a sealed value; plaintext (legacy) and "" pass
// through unchanged.
func (s *Store) decryptPayload(value string) string {
	if s.credentials == nil || value == "" {
		return value
	}
	decrypted, err := s.credentials.Decrypt(value)
	if err != nil {
		// A malformed sealed row must fail loudly rather than silently
		// wiping the payload.
		return ""
	}
	return decrypted
}

// MigrateCredentials seals every plaintext payload row in place. Run once
// at startup after SetCredentialsCipher: rows saved before the cipher
// existed stay readable (Decrypt passes plaintext through), and this
// upgrade closes the window. Idempotent — sealed rows are skipped.
func (s *Store) MigrateCredentials(ctx context.Context) error {
	if s.credentials == nil {
		return nil
	}
	var rows []MarketplaceCredential
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if !secrets.NeedsMigration(row.Payload) {
			continue
		}
		sealed, err := s.credentials.Encrypt(row.Payload)
		if err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Model(&MarketplaceCredential{}).
			Where("id = ?", row.ID).
			Update("payload", sealed).Error; err != nil {
			return err
		}
	}
	return nil
}
