package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MarketplaceSite = "site"

type SiteReviewInput struct {
	ExternalReviewID string
	SellerArticle    string
	Rating           int
	AuthorName       string
	AuthorEmail      string
	Text             string
	Pros             string
	Cons             string
	IPHash           string
	UserAgentHash    string
	Origin           string
	Referrer         string
	PrivacyConsentAt time.Time
	TermsConsentAt   time.Time
	Media            []ReviewMedia
}

type ReviewEditPatch struct {
	SellerArticle *string
	Rating        *int
	AuthorName    *string
	Text          *string
	Pros          *string
	Cons          *string
}

func NormalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", errors.New("email is required")
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address == "" || strings.Contains(addr.Address, " ") {
		return "", errors.New("email is invalid")
	}
	return strings.ToLower(addr.Address), nil
}

func HashPII(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSiteReview(ctx context.Context, input SiteReviewInput) (Review, error) {
	email, err := NormalizeEmail(input.AuthorEmail)
	if err != nil {
		return Review{}, err
	}
	if input.Rating < 1 || input.Rating > 5 {
		return Review{}, errors.New("rating must be between 1 and 5")
	}
	now := time.Now().UTC()
	if input.PrivacyConsentAt.IsZero() {
		input.PrivacyConsentAt = now
	}
	if input.TermsConsentAt.IsZero() {
		input.TermsConsentAt = now
	}
	emailHash := HashPII(email)
	var out Review
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		identity := ReviewerIdentity{
			TenantID:        DefaultTenantID,
			EmailNormalized: email,
			EmailHash:       emailHash,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "email_normalized"}},
			DoUpdates: clause.AssignmentColumns([]string{"email_hash", "updated_at"}),
		}).Create(&identity).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND email_normalized = ?", DefaultTenantID, email).First(&identity).Error; err != nil {
			return err
		}

		privacyAt := input.PrivacyConsentAt.UTC()
		termsAt := input.TermsConsentAt.UTC()
		rating := input.Rating
		review := Review{
			TenantID:           DefaultTenantID,
			Marketplace:        MarketplaceSite,
			ExternalReviewID:   input.ExternalReviewID,
			ExternalProductID:  input.SellerArticle,
			SellerArticle:      input.SellerArticle,
			ReviewerIdentityID: &identity.ID,
			Rating:             &rating,
			AuthorName:         strings.TrimSpace(input.AuthorName),
			Text:               strings.TrimSpace(input.Text),
			Pros:               strings.TrimSpace(input.Pros),
			Cons:               strings.TrimSpace(input.Cons),
			CreatedAtMP:        now,
			Status:             "pending",
			Visibility:         "hidden",
			AuthorEmailHash:    emailHash,
			SubmissionIPHash:   input.IPHash,
			SubmissionUAHash:   input.UserAgentHash,
			SubmissionOrigin:   strings.TrimSpace(input.Origin),
			SubmissionReferrer: strings.TrimSpace(input.Referrer),
			ConsentPrivacyAt:   &privacyAt,
			ConsentTermsAt:     &termsAt,
			FetchedAt:          now,
		}
		if err := tx.Create(&review).Error; err != nil {
			return err
		}
		for i := range input.Media {
			input.Media[i].ReviewID = review.ID
			if input.Media[i].Position == 0 {
				input.Media[i].Position = i + 1
			}
			if err := tx.Create(&input.Media[i]).Error; err != nil {
				return err
			}
		}
		if err := tx.Preload("ReviewerIdentity").Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		}).First(&out, review.ID).Error; err != nil {
			return err
		}
		return nil
	})
	return out, err
}

func (s *Store) SetReviewStatus(ctx context.Context, id uint, status string) error {
	updates := map[string]any{"status": status}
	switch status {
	case "approved":
		updates["visibility"] = "visible"
	case "rejected":
		updates["visibility"] = "hidden"
	case "pending":
		updates["visibility"] = "hidden"
	case "deleted":
	default:
		return errors.New("invalid review status")
	}
	return s.db.WithContext(ctx).Model(&Review{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (s *Store) UpdateReviewContent(ctx context.Context, id uint, patch ReviewEditPatch) error {
	updates := map[string]any{}
	if patch.SellerArticle != nil {
		value := strings.TrimSpace(*patch.SellerArticle)
		updates["seller_article"] = value
		updates["external_product_id"] = value
	}
	if patch.Rating != nil {
		if *patch.Rating < 1 || *patch.Rating > 5 {
			return errors.New("rating must be between 1 and 5")
		}
		updates["rating"] = *patch.Rating
	}
	if patch.AuthorName != nil {
		updates["author_name"] = strings.TrimSpace(*patch.AuthorName)
	}
	if patch.Text != nil {
		updates["text"] = strings.TrimSpace(*patch.Text)
	}
	if patch.Pros != nil {
		updates["pros"] = strings.TrimSpace(*patch.Pros)
	}
	if patch.Cons != nil {
		updates["cons"] = strings.TrimSpace(*patch.Cons)
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&Review{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) MediaByAccessToken(ctx context.Context, token string) (ReviewMedia, error) {
	var media ReviewMedia
	err := s.db.WithContext(ctx).
		Preload("Review").
		Where("access_token = ?", token).
		First(&media).Error
	return media, err
}

func (s *Store) ReviewMediaStoragePaths(ctx context.Context, id uint) ([]string, error) {
	var rows []ReviewMedia
	if err := s.db.WithContext(ctx).Where("review_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.StoragePath != "" {
			paths = append(paths, row.StoragePath)
		}
	}
	return paths, nil
}
