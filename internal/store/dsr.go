package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type SubjectExport struct {
	Email    string            `json:"email,omitempty"`
	Identity *ReviewerIdentity `json:"identity,omitempty"`
	Reviews  []Review          `json:"reviews"`
}

func (s *Store) FindSubjectByEmail(ctx context.Context, email string) (SubjectExport, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return SubjectExport{}, err
	}
	out := SubjectExport{Email: normalized, Reviews: []Review{}}

	var identity ReviewerIdentity
	err = s.db.WithContext(ctx).
		Where("tenant_id = ? AND email_normalized = ?", DefaultTenantID, normalized).
		First(&identity).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return SubjectExport{}, err
	}
	if err == nil {
		out.Identity = &identity
	}

	hash := HashPII(normalized)
	q := s.db.WithContext(ctx).Preload("Media").
		Where("tenant_id = ?", DefaultTenantID).
		Where("author_email_hash = ?", hash)
	if out.Identity != nil {
		q = s.db.WithContext(ctx).Preload("Media").
			Where("tenant_id = ?", DefaultTenantID).
			Where("author_email_hash = ? OR reviewer_identity_id = ?", hash, identity.ID)
	}
	if err := q.Order("id asc").Find(&out.Reviews).Error; err != nil {
		return SubjectExport{}, err
	}
	return out, nil
}

func (s *Store) FindReviewByExternalRef(ctx context.Context, marketplace, externalReviewID string) (SubjectExport, error) {
	out := SubjectExport{Reviews: []Review{}}
	err := s.db.WithContext(ctx).Preload("Media").
		Where("tenant_id = ? AND marketplace = ? AND external_review_id = ?", DefaultTenantID, marketplace, externalReviewID).
		Find(&out.Reviews).Error
	return out, err
}

func (s *Store) PurgeSubjectByEmail(ctx context.Context, email string) (int, error) {
	exp, err := s.FindSubjectByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, review := range exp.Reviews {
		if err := s.HardDeleteReview(ctx, review.ID); err != nil {
			return count, err
		}
		count++
	}
	if exp.Identity != nil {
		if err := s.db.WithContext(ctx).Delete(&ReviewerIdentity{}, exp.Identity.ID).Error; err != nil {
			return count, err
		}
	}
	return count, nil
}

func (s *Store) PurgeReviewByExternalRef(ctx context.Context, marketplace, externalReviewID string) (int, error) {
	exp, err := s.FindReviewByExternalRef(ctx, marketplace, externalReviewID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, review := range exp.Reviews {
		if err := s.HardDeleteReview(ctx, review.ID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Store) WriteDSRLog(ctx context.Context, log DSRLog) error {
	if log.TenantID == 0 {
		log.TenantID = DefaultTenantID
	}
	return s.db.WithContext(ctx).Create(&log).Error
}
