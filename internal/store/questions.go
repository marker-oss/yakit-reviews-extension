package store

import (
	"context"
	"time"

	"reviews/internal/auth"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuestionInput carries fields needed to upsert a marketplace-fetched question.
type QuestionInput struct {
	Marketplace        string
	ExternalQuestionID string
	ExternalProductID  string
	SellerArticle      string
	ExternalSKU        string
	AuthorName         string
	Text               string
	CreatedAtMP        time.Time
}

// QuestionFilter controls which questions ListQuestions returns.
type QuestionFilter struct {
	Marketplace   string
	Status        string
	Visibility    string
	SellerArticle string
	Limit         int
	Offset        int
}

// SiteQuestionInput carries fields for a site-submitted question.
type SiteQuestionInput struct {
	SellerArticle string
	AuthorName    string
	AuthorEmail   string
	Text          string
	IPHash        string
}

// UpsertQuestion inserts or updates a marketplace question, anonymizing the
// author name at ingestion (same posture as UpsertReview).
func (s *Store) UpsertQuestion(ctx context.Context, in QuestionInput) (Question, error) {
	now := time.Now().UTC()
	authorName := AnonymizeAuthorName(in.AuthorName)

	row := Question{
		TenantID:           DefaultTenantID,
		Marketplace:        in.Marketplace,
		ExternalQuestionID: in.ExternalQuestionID,
		ExternalProductID:  in.ExternalProductID,
		SellerArticle:      in.SellerArticle,
		ExternalSKU:        in.ExternalSKU,
		AuthorName:         authorName,
		Text:               in.Text,
		CreatedAtMP:        in.CreatedAtMP,
		Status:             "imported",
		Visibility:         "hidden",
		FetchedAt:          now,
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "marketplace"},
			{Name: "external_question_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"external_product_id",
			"seller_article",
			"external_sku",
			"author_name",
			"text",
			"created_at_mp",
			"fetched_at",
		}),
	}).Create(&row).Error
	if err != nil {
		return Question{}, err
	}

	// Reload to get the real ID (OnConflict update may not set it).
	var out Question
	err = s.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace = ? AND external_question_id = ?",
			DefaultTenantID, in.Marketplace, in.ExternalQuestionID).
		First(&out).Error
	return out, err
}

// ListQuestions returns questions matching the filter, ordered by created_at_mp desc.
func (s *Store) ListQuestions(ctx context.Context, filter QuestionFilter) ([]Question, error) {
	q := s.db.WithContext(ctx).Where("tenant_id = ?", DefaultTenantID)
	if filter.Marketplace != "" {
		q = q.Where("marketplace = ?", filter.Marketplace)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	if filter.SellerArticle != "" {
		q = q.Where("seller_article = ?", filter.SellerArticle)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	var questions []Question
	err := q.Order("created_at_mp desc").Find(&questions).Error
	return questions, err
}

// SetQuestionAnswer saves the seller's answer, marks the question answered and
// visible, and queues an answer-publish for non-site marketplaces.
func (s *Store) SetQuestionAnswer(ctx context.Context, id uint, text string) error {
	now := time.Now().UTC()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var q Question
		if err := tx.First(&q, id).Error; err != nil {
			return err
		}

		updates := map[string]any{
			"answer_text": text,
			"answer_at":   now,
			"status":      "answered",
			"visibility":  "visible",
		}
		if q.Marketplace != MarketplaceSite {
			pending := "pending"
			updates["answer_publish_state"] = pending
		}
		return tx.Model(&q).Updates(updates).Error
	})
}

// SetQuestionAnswerPublishState persists the outcome of a publish attempt.
func (s *Store) SetQuestionAnswerPublishState(ctx context.Context, id uint, state string, errText *string, publishedAt *time.Time) error {
	updates := map[string]any{
		"answer_publish_state": state,
		"answer_publish_error": errText,
		"answer_published_at":  publishedAt,
	}
	return s.db.WithContext(ctx).Model(&Question{}).Where("id = ?", id).Updates(updates).Error
}

// QuestionsNeedingAnswerPublish returns non-site questions whose answer is set
// and whose publish state is nil, pending, or failed.
func (s *Store) QuestionsNeedingAnswerPublish(ctx context.Context) ([]Question, error) {
	var questions []Question
	err := s.db.WithContext(ctx).
		Where("marketplace <> ?", MarketplaceSite).
		Where("answer_text IS NOT NULL AND answer_text <> ''").
		Where("answer_publish_state IS NULL OR answer_publish_state IN ('pending','failed')").
		Find(&questions).Error
	return questions, err
}

// QuestionByID fetches a single question by primary key.
func (s *Store) QuestionByID(ctx context.Context, id uint) (Question, error) {
	var q Question
	err := s.db.WithContext(ctx).First(&q, id).Error
	return q, err
}

// CreateSiteQuestion stores a site-submitted question as hidden/pending until
// the seller answers it.
func (s *Store) CreateSiteQuestion(ctx context.Context, in SiteQuestionInput) (Question, error) {
	token, err := auth.NewSessionToken()
	if err != nil {
		return Question{}, err
	}

	now := time.Now().UTC()
	emailHash := HashPII(in.AuthorEmail)

	q := Question{
		TenantID:           DefaultTenantID,
		Marketplace:        MarketplaceSite,
		ExternalQuestionID: "site-" + token,
		SellerArticle:      in.SellerArticle,
		AuthorName:         AnonymizeAuthorName(in.AuthorName),
		Text:               in.Text,
		Status:             "pending",
		Visibility:         "hidden",
		CreatedAtMP:        now,
		AuthorEmailHash:    emailHash,
		SubmissionIPHash:   in.IPHash,
		FetchedAt:          now,
	}

	if err := s.db.WithContext(ctx).Create(&q).Error; err != nil {
		return Question{}, err
	}
	return q, nil
}
