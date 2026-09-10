// Package scheduler runs collector jobs on a configured interval, in-process,
// so the containerized server needs no external cron or systemd timer.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Runner performs one sync tick. The runner itself resolves the current
// enabled marketplaces (e.g. from stored credentials); the scheduler holds
// no marketplace IDs.
type Runner interface {
	RunOnce(ctx context.Context)
}

type Scheduler struct {
	runner   Runner
	interval time.Duration
	logger   *slog.Logger
}

func New(runner Runner, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		runner:   runner,
		interval: interval,
		logger:   logger,
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
	s.logger.Info("scheduled sync starting")
	s.runner.RunOnce(ctx)
}
