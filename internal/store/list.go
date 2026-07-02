package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"
)

type WidgetReviewDefaults struct {
	MinRating      int    `json:"minRating"`
	RequireText    bool   `json:"requireText"`
	RequirePhoto   bool   `json:"requirePhoto"`
	Marketplace    string `json:"marketplace"`
	InitialSort    string `json:"initialSort"`
	TextFirst      bool   `json:"textFirst"`
	PhotoFirst     bool   `json:"photoFirst"`
	OnlyWithAnswer bool   `json:"onlyWithAnswer"`
}

type ReviewRankingRule struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type ReviewListFilter struct {
	Marketplace          string
	ExcludedMarketplaces []string
	Rating               int
	MinRating            int
	Limit                int
	Offset               int
	Status               string
	Visibility           string
	SellerArticle        string
	ArticleSearch        string
	HasPhoto             bool
	HasText              bool
	HasAnswer            bool
	Search               string
	PinnedFirst          bool
	SortBy               string
	Ranking              []ReviewRankingRule
}

func (s *Store) ListReviews(ctx context.Context, filter ReviewListFilter) ([]Review, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := s.db.WithContext(ctx).
		Preload("ReviewerIdentity").
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position asc").Order("id asc")
		})

	query, err := s.applyReviewFilters(query, filter)
	if err != nil {
		return nil, err
	}
	if filter.PinnedFirst {
		query = query.Order("pinned desc")
	}
	query = applyReviewOrder(query, filter).Limit(limit).Offset(filter.Offset)

	var reviews []Review
	if err := query.Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *Store) ListReviewsByWidgetRules(ctx context.Context, filter ReviewListFilter, defaults WidgetReviewDefaults, ranking []ReviewRankingRule) ([]Review, error) {
	if defaults.Marketplace != "" && defaults.Marketplace != "all" && filter.Marketplace == "" {
		filter.Marketplace = defaults.Marketplace
	}
	if defaults.MinRating > 0 && filter.Rating == 0 {
		filter.MinRating = defaults.MinRating
	}
	if defaults.RequireText {
		filter.HasText = true
	}
	if defaults.RequirePhoto {
		filter.HasPhoto = true
	}
	if defaults.OnlyWithAnswer {
		filter.HasAnswer = true
	}
	if defaults.InitialSort != "" && filter.SortBy == "" {
		filter.SortBy = defaults.InitialSort
	}
	if len(ranking) > 0 {
		filter.Ranking = filterRankingByDefaults(ranking, defaults)
	}
	return s.ListReviews(ctx, filter)
}

func (s *Store) applyReviewFilters(query *gorm.DB, filter ReviewListFilter) (*gorm.DB, error) {
	switch filter.Status {
	case "public":
		query = query.Where("status IN ?", []string{"imported", "approved"})
	case "deleted":
		query = query.Where("status = ?", "deleted")
	case "pending", "approved", "rejected", "imported":
		query = query.Where("status = ?", filter.Status)
	case "all":
	default:
		query = query.Where("status <> ?", "deleted")
	}
	if filter.Marketplace != "" && filter.Marketplace != "all" {
		query = query.Where("marketplace = ?", strings.ToLower(filter.Marketplace))
	}
	if len(filter.ExcludedMarketplaces) > 0 {
		excluded := make([]string, 0, len(filter.ExcludedMarketplaces))
		for _, marketplace := range filter.ExcludedMarketplaces {
			marketplace = strings.ToLower(strings.TrimSpace(marketplace))
			if marketplace != "" {
				excluded = append(excluded, marketplace)
			}
		}
		if len(excluded) > 0 {
			query = query.Where("LOWER(marketplace) NOT IN ?", excluded)
		}
	}
	if filter.Rating > 0 {
		if filter.Rating < 1 || filter.Rating > 5 {
			return nil, fmt.Errorf("rating must be between 1 and 5")
		}
		query = query.Where("rating = ?", filter.Rating)
	}
	if filter.MinRating > 0 {
		if filter.MinRating < 1 || filter.MinRating > 5 {
			return nil, fmt.Errorf("min rating must be between 1 and 5")
		}
		query = query.Where("rating >= ?", filter.MinRating)
	}
	if filter.Visibility != "" {
		query = query.Where("visibility = ?", filter.Visibility)
	}
	if filter.SellerArticle != "" {
		query = query.Where("seller_article = ?", filter.SellerArticle)
	}
	if filter.ArticleSearch != "" {
		raw := strings.TrimSpace(filter.ArticleSearch)
		like := "%" + strings.ToLower(raw) + "%"
		rawLike := "%" + raw + "%"
		normalized := stripArticleSearchSeparators(raw)
		normalizedLower := strings.ToLower(normalized)
		normalizedLike := "%" + normalized + "%"
		normalizedLowerLike := "%" + normalizedLower + "%"
		query = query.Where(
			`seller_article LIKE ?
				OR external_product_id LIKE ?
				OR LOWER(seller_article) LIKE ?
				OR LOWER(external_product_id) LIKE ?
				OR REPLACE(REPLACE(REPLACE(REPLACE(seller_article, '/', ''), '-', ''), '_', ''), ' ', '') LIKE ?
				OR REPLACE(REPLACE(REPLACE(REPLACE(external_product_id, '/', ''), '-', ''), '_', ''), ' ', '') LIKE ?
				OR LOWER(REPLACE(REPLACE(REPLACE(REPLACE(seller_article, '/', ''), '-', ''), '_', ''), ' ', '')) LIKE ?
				OR LOWER(REPLACE(REPLACE(REPLACE(REPLACE(external_product_id, '/', ''), '-', ''), '_', ''), ' ', '')) LIKE ?`,
			rawLike, rawLike, like, like, normalizedLike, normalizedLike, normalizedLowerLike, normalizedLowerLike,
		)
	}
	if filter.HasPhoto {
		query = query.Where("id IN (?)",
			s.db.Model(&ReviewMedia{}).Select("review_id").Where("kind = ?", "photo"))
	}
	if filter.HasText {
		query = query.Where("LENGTH(TRIM(text)) > 0")
	}
	if filter.HasAnswer {
		query = query.Where("(mp_answer_text IS NOT NULL AND LENGTH(TRIM(mp_answer_text)) > 0) OR (admin_reply_text IS NOT NULL AND LENGTH(TRIM(admin_reply_text)) > 0)")
	}
	if filter.Search != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(filter.Search)) + "%"
		query = query.Where(
			`LOWER(text) LIKE ?
				OR LOWER(author_name) LIKE ?
				OR LOWER(pros) LIKE ?
				OR LOWER(cons) LIKE ?
				OR LOWER(external_review_id) LIKE ?
				OR LOWER(mp_answer_text) LIKE ?
				OR LOWER(admin_reply_text) LIKE ?`,
			like, like, like, like, like, like, like,
		)
	}
	return query, nil
}

func stripArticleSearchSeparators(value string) string {
	return strings.NewReplacer("/", "", "-", "", "_", "", " ", "").Replace(strings.TrimSpace(value))
}

func applyReviewOrder(query *gorm.DB, filter ReviewListFilter) *gorm.DB {
	if len(filter.Ranking) > 0 || filter.SortBy == "relevance" {
		ranking := filter.Ranking
		if len(ranking) == 0 {
			ranking = DefaultReviewRanking()
		}
		for _, rule := range ranking {
			if order, ok := rankingOrder(rule); ok {
				query = query.Order(order)
			}
		}
		return query.Order("created_at_mp desc")
	}
	switch filter.SortBy {
	case "highest":
		return query.Order("rating desc").Order("created_at_mp desc")
	case "lowest":
		return query.Order("rating asc").Order("created_at_mp desc")
	case "media":
		return query.Order("CASE WHEN id IN (SELECT review_id FROM review_media WHERE kind = 'photo') THEN 1 ELSE 0 END desc").Order("created_at_mp desc")
	default:
		return query.Order("created_at_mp desc")
	}
}

func DefaultReviewRanking() []ReviewRankingRule {
	return []ReviewRankingRule{
		{Field: "pinned", Direction: "desc"},
		{Field: "hasPhoto", Direction: "desc"},
		{Field: "hasText", Direction: "desc"},
		{Field: "rating", Direction: "desc"},
		{Field: "createdAt", Direction: "desc"},
	}
}

func DefaultWidgetReviewDefaults() WidgetReviewDefaults {
	return WidgetReviewDefaults{
		MinRating:   4,
		RequireText: true,
		InitialSort: "relevance",
		TextFirst:   true,
		PhotoFirst:  true,
		Marketplace: "all",
	}
}

func rankingOrder(rule ReviewRankingRule) (string, bool) {
	direction := strings.ToLower(rule.Direction)
	if !slices.Contains([]string{"asc", "desc"}, direction) {
		direction = "desc"
	}
	switch rule.Field {
	case "pinned":
		return "pinned " + direction, true
	case "hasPhoto":
		return "CASE WHEN id IN (SELECT review_id FROM review_media WHERE kind = 'photo') THEN 1 ELSE 0 END " + direction, true
	case "hasText":
		return "CASE WHEN LENGTH(TRIM(text)) > 0 THEN 1 ELSE 0 END " + direction, true
	case "rating":
		return "rating " + direction, true
	case "createdAt":
		return "created_at_mp " + direction, true
	default:
		return "", false
	}
}

func filterRankingByDefaults(ranking []ReviewRankingRule, defaults WidgetReviewDefaults) []ReviewRankingRule {
	if defaults.PhotoFirst && defaults.TextFirst {
		return ranking
	}
	filtered := make([]ReviewRankingRule, 0, len(ranking))
	for _, rule := range ranking {
		if rule.Field == "hasPhoto" && !defaults.PhotoFirst {
			continue
		}
		if rule.Field == "hasText" && !defaults.TextFirst {
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered
}
