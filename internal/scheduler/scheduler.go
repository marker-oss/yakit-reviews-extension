// Package scheduler runs collector jobs on a configured interval, in-process,
// so the containerized server needs no external cron or systemd timer.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Runner is the subset of collector.Runner the scheduler depends on.
type Runner interface {
	RunOnce(ctx context.Context, marketplaces []string)
}

type Scheduler struct {
	runner       Runner
	interval     time.Duration
	marketplaces []string
	logger       *slog.Logger
}

func New(runner Runner, interval time.Duration, marketplaces []string, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		runner:       runner,
		interval:     interval,
		marketplaces: marketplaces,
		logger:       logger,
	}
}

// Run blocks until ctx is cancelled. It runs one sync immediately, then once
// per interval.
func (s *Scheduler) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	s.runOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.logger.Info("scheduled sync starting", "marketplaces", s.marketplaces)
	s.runner.RunOnce(ctx, s.marketplaces)
}
