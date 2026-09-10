package store

import (
	"context"
	"testing"
	"time"

	"reviews/internal/marketplace"
)

func TestReplyPublishStateAndQueue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rating := 5
	res, err := s.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-1", ExternalProductID: "p1",
		Rating: &rating, Text: "ok", CreatedAtMP: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	id := res.Review.ID

	// A review with a reply but no publish state is queued.
	reply := "Спасибо!"
	if err := s.SetReviewReply(ctx, id, &reply); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := s.SetReplyPublishState(ctx, id, "pending", nil, nil); err != nil {
		t.Fatalf("set pending: %v", err)
	}
	queued, err := s.ReviewsNeedingReplyPublish(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(queued) != 1 || queued[0].ID != id {
		t.Fatalf("expected 1 queued, got %+v", queued)
	}

	// Marking published removes it from the queue and records the timestamp.
	now := time.Now().UTC()
	if err := s.SetReplyPublishState(ctx, id, "published", nil, &now); err != nil {
		t.Fatalf("set published: %v", err)
	}
	queued, _ = s.ReviewsNeedingReplyPublish(ctx)
	if len(queued) != 0 {
		t.Fatalf("expected empty queue, got %d", len(queued))
	}
	got, err := s.ReviewByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" || got.ReplyPublishedAt == nil {
		t.Fatalf("unexpected state: %+v", got)
	}
}
