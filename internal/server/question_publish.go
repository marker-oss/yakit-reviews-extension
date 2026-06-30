package server

import (
	"context"
	"time"

	"reviews/internal/store"
)

// publishQuestionAnswer attempts to publish a saved answer to the marketplace
// and records the outcome on the question. Site questions and marketplaces
// without a publisher are marked "unsupported". Reuses the same per-marketplace
// toggle as reply publishing (store.PublishRepliesKey).
func (s *Server) publishQuestionAnswer(ctx context.Context, q store.Question) {
	id := q.ID
	if q.Marketplace == store.MarketplaceSite {
		s.setQuestionUnsupported(ctx, id)
		return
	}
	pub, ok := s.questionAnswerPublishers[q.Marketplace]
	if !ok || !s.replyPublishEnabled(ctx, q.Marketplace) {
		s.setQuestionUnsupported(ctx, id)
		return
	}
	text := ""
	if q.AnswerText != nil {
		text = *q.AnswerText
	}
	if err := pub.PublishQuestionAnswer(ctx, q.ExternalQuestionID, q.ExternalSKU, text); err != nil {
		msg := err.Error()
		_ = s.store.SetQuestionAnswerPublishState(ctx, id, "failed", &msg, nil)
		s.logger.Warn("publish question answer failed", "question", id, "marketplace", q.Marketplace, "error", err)
		return
	}
	now := time.Now().UTC()
	_ = s.store.SetQuestionAnswerPublishState(ctx, id, "published", nil, &now)
}

func (s *Server) setQuestionUnsupported(ctx context.Context, id uint) {
	_ = s.store.SetQuestionAnswerPublishState(ctx, id, "unsupported", nil, nil)
}

// RetryPendingQuestionAnswers re-attempts every queued question answer. The
// queue includes questions with NULL publish state (never attempted) as well as
// explicit "failed" state. Called at the end of each sync run alongside
// RetryPendingReplies.
func (s *Server) RetryPendingQuestionAnswers(ctx context.Context) {
	rows, err := s.store.QuestionsNeedingAnswerPublish(ctx)
	if err != nil {
		s.logger.Error("load pending question answers", "error", err)
		return
	}
	for _, q := range rows {
		s.publishQuestionAnswer(ctx, q)
	}
}
