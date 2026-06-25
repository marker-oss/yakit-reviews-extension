package store

import (
	"context"
	"testing"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func TestScrubPersonalData(t *testing.T) {
	s := newTestStore(t)
	// Insert a legacy row directly: full name, populated Raw, empty SellerArticle.
	s.db.Create(&Review{
		Marketplace: "wb", ExternalReviewID: "legacy1",
		AuthorName: "Анна Котова", SellerArticle: "",
		Raw: `{"productDetails":{"supplierArticle":"ART-9"},"userName":"Анна Котова"}`,
	})
	n, err := s.ScrubPersonalData(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("scrubbed = %d, want 1", n)
	}
	var got Review
	s.db.Where("external_review_id = ?", "legacy1").First(&got)
	if got.AuthorName != "Анна К." {
		t.Errorf("AuthorName = %q", got.AuthorName)
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
	if got.SellerArticle != "ART-9" {
		t.Errorf("SellerArticle = %q, want ART-9", got.SellerArticle)
	}
	// Idempotent: second run scrubs nothing.
	n2, _ := s.ScrubPersonalData(testCtx(t))
	if n2 != 0 {
		t.Errorf("second run scrubbed = %d, want 0", n2)
	}
}

func TestSupplierArticleFromRaw(t *testing.T) {
	raw := `{"productDetails":{"supplierArticle":"ART-1"},"userName":"Анна Котова"}`
	if got := supplierArticleFromRaw(raw); got != "ART-1" {
		t.Errorf("got %q, want ART-1", got)
	}
	if got := supplierArticleFromRaw(`{}`); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := supplierArticleFromRaw(``); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
