package server

import (
	"context"

	"reviews/internal/marketplace"
)

// SyncDispatch reports which marketplaces a sync trigger started versus
// which were already busy (an in-flight sync for that marketplace already
// held the coordinator slot).
type SyncDispatch struct {
	Started []string `json:"started"`
	Busy    []string `json:"busy"`
}

// TriggerSyncFunc validates and dispatches a marketplace sync. It returns
// once every requested marketplace is either started (background work) or
// rejected as busy; it never blocks on the sync itself.
type TriggerSyncFunc func(marketplaces []string) (SyncDispatch, error)

// ReplyPublisherResolver returns a fresh reply publisher for a marketplace,
// reading current (possibly just-changed) credentials.
type ReplyPublisherResolver func(ctx context.Context, marketplaceID string) (marketplace.ReplyPublisher, error)

// QuestionPublisherResolver returns a fresh question-answer publisher.
type QuestionPublisherResolver func(ctx context.Context, marketplaceID string) (marketplace.QuestionAnswerPublisher, error)
