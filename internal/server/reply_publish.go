package server

import (
	"context"
	"time"

	"reviews/internal/store"
)

// replyPublishEnabled reports whether replies should be auto-published for a
// marketplace. Default when unset: WB and YM on, Ozon off.
func (s *Server) replyPublishEnabled(ctx context.Context, marketplace string) bool {
	value, err := s.store.GetAppSetting(ctx, store.PublishRepliesKey(marketplace))
	if err == nil && value != "" {
		return value == "true"
	}
	return marketplace != "ozon"
}

// publishReply attempts to publish a saved reply to the marketplace and records
// the outcome on the review. Site reviews and marketplaces without a publisher
// are marked "unsupported".
func (s *Server) publishReply(ctx context.Context, review store.Review) {
	id := review.ID
	if review.Marketplace == store.MarketplaceSite {
		s.setUnsupported(ctx, id)
		return
	}
	pub, ok := s.replyPublishers[review.Marketplace]
	if !ok || !s.replyPublishEnabled(ctx, review.Marketplace) {
		s.setUnsupported(ctx, id)
		return
	}
	text := ""
	if review.AdminReplyText != nil {
		text = *review.AdminReplyText
	}
	if err := pub.PublishReply(ctx, review.ExternalReviewID, text); err != nil {
		msg := err.Error()
		_ = s.store.SetReplyPublishState(ctx, id, "failed", &msg, nil)
		s.logger.Warn("publish reply failed", "review", id, "marketplace", review.Marketplace, "error", err)
		return
	}
	now := time.Now().UTC()
	_ = s.store.SetReplyPublishState(ctx, id, "published", nil, &now)
}

func (s *Server) setUnsupported(ctx context.Context, id uint) {
	_ = s.store.SetReplyPublishState(ctx, id, "unsupported", nil, nil)
}

// RetryPendingReplies re-attempts every queued reply. The queue includes
// reviews with NULL publish state (never attempted) as well as explicit
// "failed" state — it does NOT re-publish already "published" replies.
// Called at the end of each sync run.
func (s *Server) RetryPendingReplies(ctx context.Context) {
	rows, err := s.store.ReviewsNeedingReplyPublish(ctx)
	if err != nil {
		s.logger.Error("load pending replies", "error", err)
		return
	}
	for _, review := range rows {
		s.publishReply(ctx, review)
	}
}
