package store

import (
	"context"
	"errors"
	"time"

	"reviews/internal/marketplace"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UpsertResult struct {
	Created bool
	Review  Review
}

func (s *Store) UpsertReview(ctx context.Context, input marketplace.Review) (UpsertResult, error) {
	var result UpsertResult

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		productID, err := resolveProductID(tx, DefaultTenantID, input.Marketplace, input.ExternalProductID)
		if err != nil {
			return err
		}

		answerText, answerState := answerFields(input.Answer)
		now := time.Now().UTC()
		next := Review{
			TenantID:          DefaultTenantID,
			Marketplace:       input.Marketplace,
			ExternalReviewID:  input.ExternalReviewID,
			ExternalProductID: input.ExternalProductID,
			SellerArticle:     input.SellerArticle,
			ProductID:         productID,
			Rating:            input.Rating,
			AuthorName:        input.AuthorName,
			Text:              input.Text,
			Pros:              input.Pros,
			Cons:              input.Cons,
			CreatedAtMP:       input.CreatedAtMP,
			UpdatedAtMP:       input.UpdatedAtMP,
			MPAnswerText:      answerText,
			MPAnswerState:     answerState,
			Status:            "imported",
			Raw:               string(input.Raw),
			FetchedAt:         now,
		}

		var existing Review
		err = tx.Where(
			"tenant_id = ? AND marketplace = ? AND external_review_id = ?",
			DefaultTenantID,
			input.Marketplace,
			input.ExternalReviewID,
		).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(&next).Error; err != nil {
				return err
			}
			result.Created = true
			result.Review = next
		case err != nil:
			return err
		default:
			updates := map[string]any{
				"external_product_id": input.ExternalProductID,
				"seller_article":      input.SellerArticle,
				"product_id":          productID,
				"rating":              input.Rating,
				"author_name":         input.AuthorName,
				"text":                input.Text,
				"pros":                input.Pros,
				"cons":                input.Cons,
				"created_at_mp":       input.CreatedAtMP,
				"updated_at_mp":       input.UpdatedAtMP,
				"mp_answer_text":      answerText,
				"mp_answer_state":     answerState,
				"raw":                 string(input.Raw),
				"fetched_at":          now,
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.First(&existing, existing.ID).Error; err != nil {
				return err
			}
			result.Review = existing
		}

		return replaceMedia(tx, result.Review.ID, input.Media)
	})

	return result, err
}

func resolveProductID(tx *gorm.DB, tenantID uint, marketplaceID, externalProductID string) (*uint, error) {
	var link ProductMarketplaceLink
	err := tx.Where(
		"tenant_id = ? AND marketplace = ? AND external_product_id = ?",
		tenantID,
		marketplaceID,
		externalProductID,
	).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &link.ProductID, nil
}

func replaceMedia(tx *gorm.DB, reviewID uint, media []marketplace.Media) error {
	if len(media) == 0 {
		return tx.Where("review_id = ?", reviewID).Delete(&ReviewMedia{}).Error
	}

	urls := make([]string, 0, len(media))
	for _, item := range media {
		urls = append(urls, item.URL)
	}

	if err := tx.Where("review_id = ? AND url NOT IN ?", reviewID, urls).Delete(&ReviewMedia{}).Error; err != nil {
		return err
	}

	for _, item := range media {
		row := ReviewMedia{
			ReviewID:   reviewID,
			Kind:       item.Kind,
			URL:        item.URL,
			PreviewURL: emptyStringAsNil(item.PreviewURL),
			Position:   item.Position,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "review_id"}, {Name: "url"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"kind",
				"preview_url",
				"position",
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}

	return nil
}

func answerFields(answer *marketplace.Answer) (*string, *string) {
	if answer == nil {
		return nil, nil
	}
	return &answer.Text, &answer.State
}

func emptyStringAsNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
