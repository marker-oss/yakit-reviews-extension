package store

import (
	"context"
	"testing"

	"reviews/internal/marketplace"
)

func TestAnonymizeAuthorName(t *testing.T) {
	cases := map[string]string{
		"Анна Котова":      "Анна К.",
		"анна котова":      "анна к.",
		"Иван":             "Иван",
		"":                 "",
		"  Пётр  Сидоров ": "Пётр С.",
		"Mary Jane Watson": "Mary J.",
		"Анна К.":          "Анна К.", // already anonymized — idempotent
	}
	for in, want := range cases {
		if got := AnonymizeAuthorName(in); got != want {
			t.Errorf("AnonymizeAuthorName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUpsertReviewAnonymizesAndDropsRaw(t *testing.T) {
	s := newTestStore(t)
	_, err := s.UpsertReview(context.Background(), marketplace.Review{
		Marketplace:      "wb",
		ExternalReviewID: "r1",
		AuthorName:       "Анна Котова",
		Text:             "ok",
		Raw:              []byte(`{"userName":"Анна Котова"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got Review
	if err := s.db.Where("external_review_id = ?", "r1").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.AuthorName != "Анна К." {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, "Анна К.")
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
}

func TestUpsertReviewAnonymizesOnUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := marketplace.Review{
		Marketplace:      "wb",
		ExternalReviewID: "r1",
		AuthorName:       "Анна Котова",
		Text:             "ok",
		Raw:              []byte(`{"userName":"Анна Котова"}`),
	}
	if _, err := s.UpsertReview(ctx, base); err != nil {
		t.Fatal(err)
	}

	// Re-ingest the same review (same Marketplace+ExternalReviewID) with a new
	// full name and a non-empty Raw — this exercises the UPDATE branch.
	updated := base
	updated.AuthorName = "Борис Петров"
	updated.Raw = []byte(`{"userName":"Борис Петров"}`)
	if _, err := s.UpsertReview(ctx, updated); err != nil {
		t.Fatal(err)
	}

	var got Review
	if err := s.db.Where("external_review_id = ?", "r1").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.AuthorName != "Борис П." {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, "Борис П.")
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
}
