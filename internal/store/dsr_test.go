package store

import (
	"context"
	"testing"
	"time"
)

func TestFindAndPurgeSubjectByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.CreateSiteReview(ctx, SiteReviewInput{
		ExternalReviewID: "site-1", SellerArticle: "a1", Rating: 5,
		AuthorName: "Иван", AuthorEmail: "Ivan@Example.com", Text: "отлично",
	})
	if err != nil {
		t.Fatalf("create site review: %v", err)
	}

	exp, err := s.FindSubjectByEmail(ctx, "ivan@example.com")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if exp.Identity == nil || len(exp.Reviews) != 1 {
		t.Fatalf("expected identity + 1 review, got %+v", exp)
	}

	// Unknown email → empty, no error.
	empty, err := s.FindSubjectByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("find unknown: %v", err)
	}
	if empty.Identity != nil || len(empty.Reviews) != 0 {
		t.Fatalf("expected empty for unknown, got %+v", empty)
	}

	// Purge removes reviews + identity; idempotent.
	n, err := s.PurgeSubjectByEmail(ctx, "ivan@example.com")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purge count = %d, want 1", n)
	}
	again, err := s.FindSubjectByEmail(ctx, "ivan@example.com")
	if err != nil {
		t.Fatalf("find after purge: %v", err)
	}
	if again.Identity != nil || len(again.Reviews) != 0 {
		t.Fatalf("expected empty after purge, got %+v", again)
	}
	n2, _ := s.PurgeSubjectByEmail(ctx, "ivan@example.com")
	if n2 != 0 {
		t.Fatalf("second purge count = %d, want 0", n2)
	}
}

func TestDSRLogWrite(t *testing.T) {
	s := newTestStore(t)
	if err := s.WriteDSRLog(context.Background(), DSRLog{
		EmailHash: HashPII("a@b.co"), Action: "export", AdminUserID: 1, At: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("log: %v", err)
	}
}
