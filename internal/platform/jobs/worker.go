package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	leaseDuration  = 30 * time.Second
	heartbeatEvery = 10 * time.Second
	workDeadline   = 5 * time.Minute
)

type Handler func(context.Context, Claim) (objectKey string, err error)

func (q Queue) Run(ctx context.Context, worker string, handlers map[string]Handler, logger *slog.Logger) error {
	if len(handlers) == 0 {
		logger.Info("worker is idle; no job handlers are registered")
		for ctx.Err() == nil {
			if err := q.RecoverExhausted(ctx); err != nil {
				logger.ErrorContext(ctx, "job lease recovery failed", "error", err)
			}
			if wait(ctx, 30*time.Second) != nil {
				return nil
			}
		}
		return nil
	}
	types := make([]string, 0, len(handlers))
	for jobType := range handlers {
		types = append(types, jobType)
	}
	sort.Strings(types)
	for ctx.Err() == nil {
		if err := q.RecoverExhausted(ctx); err != nil {
			logger.ErrorContext(ctx, "job lease recovery failed", "error", err)
		}
		claim, err := q.ClaimTypes(ctx, worker, leaseDuration, types)
		if errors.Is(err, pgx.ErrNoRows) {
			if err = wait(ctx, 500*time.Millisecond); err != nil {
				return nil
			}
			continue
		}
		if err != nil {
			logger.ErrorContext(ctx, "job claim failed", "error", err)
			if err = wait(ctx, time.Second); err != nil {
				return nil
			}
			continue
		}
		q.runClaim(ctx, claim, handlers[claim.Type], logger)
	}
	return nil
}

func (q Queue) runClaim(ctx context.Context, claim Claim, handler Handler, logger *slog.Logger) {
	workCtx, cancel := context.WithTimeout(ctx, workDeadline)
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				heartbeatDone <- nil
				return
			case <-workCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				heartbeatCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
				err := q.Heartbeat(heartbeatCtx, claim, leaseDuration)
				done()
				if err != nil {
					heartbeatDone <- err
					cancel()
					return
				}
			}
		}
	}()

	objectKey, err := handler(workCtx, claim)
	close(stopHeartbeat)
	heartbeatErr := <-heartbeatDone
	cancel()
	if heartbeatErr != nil {
		logger.WarnContext(ctx, "job lease lost", "job_id", claim.ID, "job_type", claim.Type)
		return
	}
	if err == nil && claim.Type == "snapshot" && objectKey == "" {
		err = errors.New("snapshot handler completed without an object key")
	}
	if err != nil {
		if retryErr := q.RetryWithCode(context.Background(), claim, retryDelay(claim.Attempt), "handler_failed"); retryErr != nil {
			logger.ErrorContext(ctx, "job retry failed", "job_id", claim.ID, "job_type", claim.Type, "error", retryErr)
		}
		logger.WarnContext(ctx, "job handler failed", "job_id", claim.ID, "job_type", claim.Type)
		return
	}
	if err := q.Complete(ctx, claim, objectKey); err != nil {
		logger.WarnContext(ctx, "job completion rejected", "job_id", claim.ID, "job_type", claim.Type, "error", err)
		return
	}
	logger.InfoContext(ctx, "job completed", "job_id", claim.ID, "job_type", claim.Type)
}

func retryDelay(attempt int) time.Duration {
	delay := 500 * time.Millisecond * time.Duration(1<<min(max(attempt-1, 0), 6))
	return delay/2 + time.Duration(rand.Int64N(int64(delay)))
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
