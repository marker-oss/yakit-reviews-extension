package store

import (
	"context"
	"encoding/json"
)

func supplierArticleFromRaw(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed struct {
		ProductDetails struct {
			SupplierArticle string `json:"supplierArticle"`
		} `json:"productDetails"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	return parsed.ProductDetails.SupplierArticle
}

// ScrubPersonalData removes stored personal data from existing rows:
// anonymizes AuthorName, backfills SellerArticle from Raw when missing, and
// clears Raw. Idempotent — rows already clean (Raw empty and name anonymized)
// are skipped, so it is safe to run on every startup.
func (s *Store) ScrubPersonalData(ctx context.Context) (int, error) {
	var rows []Review
	// Candidates: any row still holding Raw.
	if err := s.db.WithContext(ctx).
		Where("raw <> ''").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	scrubbed := 0
	for _, row := range rows {
		updates := map[string]any{"raw": ""}
		anon := AnonymizeAuthorName(row.AuthorName)
		if anon != row.AuthorName {
			updates["author_name"] = anon
		}
		if row.SellerArticle == "" {
			if art := supplierArticleFromRaw(row.Raw); art != "" {
				updates["seller_article"] = art
			}
		}
		if err := s.db.WithContext(ctx).Model(&Review{}).
			Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return scrubbed, err
		}
		scrubbed++
	}
	return scrubbed, nil
}
