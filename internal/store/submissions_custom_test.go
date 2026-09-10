package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCreateSiteReviewCustomDataRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	custom := map[string]string{"height": "160-170", "fit": "в размер"}
	encoded, err := json.Marshal(custom)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	review, err := s.CreateSiteReview(ctx, SiteReviewInput{
		ExternalReviewID: "site-custom-1",
		SellerArticle:    "1523",
		Rating:           5,
		AuthorName:       "Анна",
		AuthorEmail:      "custom@example.com",
		Text:             "Село отлично",
		CustomData:       string(encoded),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(review.CustomData), &got); err != nil {
		t.Fatalf("unmarshal stored CustomData %q: %v", review.CustomData, err)
	}
	if len(got) != 2 || got["height"] != "160-170" || got["fit"] != "в размер" {
		t.Fatalf("stored custom mismatch: %+v", got)
	}

	// Legacy path: no custom data at all.
	plain, err := s.CreateSiteReview(ctx, SiteReviewInput{
		ExternalReviewID: "site-plain-2",
		SellerArticle:    "1523",
		Rating:           4,
		AuthorName:       "Ирина",
		AuthorEmail:      "plain@example.com",
		Text:             "Без допполей",
	})
	if err != nil {
		t.Fatalf("create plain: %v", err)
	}
	if plain.CustomData != "" {
		t.Fatalf("empty input must store empty CustomData, got %q", plain.CustomData)
	}
}
