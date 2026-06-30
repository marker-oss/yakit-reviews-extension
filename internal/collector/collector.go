package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/store"
)

type Runner struct {
	store    *store.Store
	cfg      config.SyncConfig
	logger   *slog.Logger
	adapters map[string]marketplace.Adapter
}

type Result struct {
	Marketplace string
	Seen        int
	Upserted    int
	Error       error
}

func NewRunner(store *store.Store, cfg config.SyncConfig, logger *slog.Logger, adapters []marketplace.Adapter) *Runner {
	byMarketplace := make(map[string]marketplace.Adapter, len(adapters))
	for _, adapter := range adapters {
		byMarketplace[adapter.Marketplace()] = adapter
	}
	return &Runner{
		store:    store,
		cfg:      cfg,
		logger:   logger,
		adapters: byMarketplace,
	}
}

func (r *Runner) RunOnce(ctx context.Context, marketplaces []string) []Result {
	results := make([]Result, 0, len(marketplaces))
	for _, marketplaceID := range marketplaces {
		result := r.runMarketplace(ctx, marketplaceID)
		results = append(results, result)
	}
	return results
}

func (r *Runner) runMarketplace(ctx context.Context, marketplaceID string) Result {
	result := Result{Marketplace: marketplaceID}
	adapter, ok := r.adapters[marketplaceID]
	if !ok {
		result.Error = fmt.Errorf("adapter %s is not implemented", marketplaceID)
		return result
	}

	startedAt := time.Now().UTC()
	run, err := r.store.CreateSyncRun(ctx, marketplaceID, startedAt)
	if err != nil {
		result.Error = err
		return result
	}

	status := "ok"
	var errText *string
	defer func() {
		if result.Error != nil {
			status = "error"
			text := result.Error.Error()
			errText = &text
		}
		if err := r.store.FinishSyncRun(ctx, run.ID, status, result.Seen, result.Upserted, errText); err != nil {
			r.logger.Error("finish sync run", "marketplace", marketplaceID, "error", err)
		}
	}()

	state, err := r.store.GetSyncState(ctx, marketplaceID)
	if err != nil {
		result.Error = err
		return result
	}

	since := startedAt.AddDate(0, -r.cfg.BackfillMonths, 0)
	if state.LastSyncedAt != nil {
		since = state.LastSyncedAt.Add(-r.cfg.Overlap)
	}

	var watermark time.Time
	cursor := ""
	for {
		reviews, nextCursor, err := adapter.FetchReviews(ctx, since, cursor)
		if err != nil {
			result.Error = err
			return result
		}

		for _, review := range reviews {
			result.Seen++
			upsert, err := r.store.UpsertReview(ctx, review)
			if err != nil {
				result.Error = err
				return result
			}
			if upsert.Created {
				result.Upserted++
			}
			watermark = maxReviewTime(watermark, review, startedAt)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	// Fetch questions if the adapter supports it. Non-fatal: errors are logged
	// and do not fail the review sync.
	if qf, ok := adapter.(marketplace.QuestionFetcher); ok {
		qCursor := ""
		for {
			questions, nextQCursor, qErr := qf.FetchQuestions(ctx, since, qCursor)
			if qErr != nil {
				r.logger.Warn("fetch questions failed", "marketplace", marketplaceID, "error", qErr)
				break
			}
			for _, q := range questions {
				if _, uErr := r.store.UpsertQuestion(ctx, store.QuestionInput{
					Marketplace:        marketplaceID,
					ExternalQuestionID: q.ExternalQuestionID,
					ExternalProductID:  q.ExternalProductID,
					SellerArticle:      q.SellerArticle,
					ExternalSKU:        q.ExternalSKU,
					AuthorName:         q.AuthorName,
					Text:               q.Text,
					CreatedAtMP:        q.CreatedAtMP,
				}); uErr != nil {
					r.logger.Warn("upsert question failed", "marketplace", marketplaceID, "question", q.ExternalQuestionID, "error", uErr)
				}
			}
			if nextQCursor == "" {
				break
			}
			qCursor = nextQCursor
		}
	}

	if watermark.IsZero() {
		watermark = startedAt
	}
	backfilled := state.Backfilled || state.LastSyncedAt == nil
	if err := r.store.SaveSyncState(ctx, store.SyncState{
		Marketplace:  marketplaceID,
		LastSyncedAt: &watermark,
		Backfilled:   backfilled,
	}); err != nil {
		result.Error = err
		return result
	}

	r.logger.Info("sync marketplace complete",
		"marketplace", marketplaceID,
		"seen", result.Seen,
		"upserted", result.Upserted,
		"watermark", watermark.Format(time.RFC3339))
	return result
}

func maxReviewTime(current time.Time, review marketplace.Review, upperBound time.Time) time.Time {
	candidate := review.CreatedAtMP
	if review.UpdatedAtMP != nil {
		candidate = *review.UpdatedAtMP
	}
	if candidate.After(upperBound) {
		candidate = upperBound
	}
	if current.IsZero() || candidate.After(current) {
		return candidate
	}
	return current
}
