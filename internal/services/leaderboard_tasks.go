package services

import (
	"ai-later-nav/internal/models"
	"context"
	"log"
	"net/http"
	"time"
)

type leaderboardTaskStore interface {
	LeaderboardStore
	ReplaceManual(models.Leaderboard) error
}

// LeaderboardTasks owns the BenchLM import, its schedule, and background reconciliation.
type LeaderboardTasks struct {
	Scheduler *CronTaskScheduler
	board     leaderboardTaskStore
	client    *http.Client
	endpoint  string
	cancel    context.CancelFunc
}

func NewLeaderboardTasks(board leaderboardTaskStore, tasks CronTaskStore, client *http.Client, endpoint string) *LeaderboardTasks {
	r := &LeaderboardTasks{board: board, client: client, endpoint: endpoint}
	r.Scheduler = NewCronTaskScheduler(tasks, map[string]func(context.Context) error{
		BenchLMTaskKey: func(ctx context.Context) error { return r.sync(ctx, false) },
	})
	r.Scheduler.SetManualJob(BenchLMTaskKey, func(ctx context.Context) error { return r.sync(ctx, true) })
	return r
}

func (r *LeaderboardTasks) sync(ctx context.Context, manual bool) error {
	wait := func(ctx context.Context, delay time.Duration) error {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	fetch := func(ctx context.Context) (models.Leaderboard, error) {
		return FetchLeaderboard(ctx, r.client, r.endpoint)
	}
	return RetrySync(ctx, func(ctx context.Context) error {
		if manual {
			board, err := fetch(ctx)
			if err != nil {
				return err
			}
			return r.board.ReplaceManual(board)
		}
		return SyncIfDue(ctx, r.board, time.Now(), fetch)
	}, wait)
}

// Start loads the persisted cron configuration and reconciles changes from other instances.
func (r *LeaderboardTasks) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	r.Scheduler.SetContext(ctx)
	if err := r.Scheduler.Start(); err != nil {
		cancel()
		return err
	}
	r.cancel = cancel
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Scheduler.Reconcile(); err != nil {
					log.Printf("Reload cron tasks: %v", err)
				}
			}
		}
	}()
	return nil
}

func (r *LeaderboardTasks) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.Scheduler.Stop()
}
