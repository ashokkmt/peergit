package outbox

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
	workerLease = 6 * time.Minute
	workTimeout = 5 * time.Minute
)

// A handler must make downstream work durable and idempotent by EventID before it returns nil.
type Handler func(context.Context, Delivery) error

func (q Queue) Run(ctx context.Context, worker string, handlers map[string]Handler, logger *slog.Logger) error {
	if worker == "" {
		return errors.New("worker name is required")
	}
	if len(handlers) == 0 {
		logger.Info("outbox worker is idle; no handlers are registered")
		for ctx.Err() == nil {
			if err := q.RecoverExhausted(ctx); err != nil {
				logger.ErrorContext(ctx, "outbox lease recovery failed", "error", err)
			}
			if wait(ctx, 30*time.Second) != nil {
				return nil
			}
		}
		return nil
	}
	names := make([]string, 0, len(handlers))
	for name := range handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	for ctx.Err() == nil {
		if err := q.RecoverExhausted(ctx); err != nil {
			logger.ErrorContext(ctx, "outbox lease recovery failed", "error", err)
		}
		worked := false
		for _, name := range names {
			delivery, err := q.Claim(ctx, name, worker, workerLease)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				logger.ErrorContext(ctx, "outbox claim failed", "handler", name, "error", err)
				continue
			}
			worked = true
			workCtx, cancel := context.WithTimeout(ctx, workTimeout)
			err = handlers[name](workCtx, delivery)
			cancel()
			if err != nil {
				if retryErr := q.Retry(context.Background(), delivery, retryDelay(delivery.Attempt), "handler_failed"); retryErr != nil {
					logger.ErrorContext(ctx, "outbox retry failed", "event_id", delivery.EventID, "handler", name, "error", retryErr)
				}
				logger.WarnContext(ctx, "outbox handler failed", "event_id", delivery.EventID, "event_type", delivery.EventType, "handler", name)
				continue
			}
			if err := q.Complete(ctx, delivery); err != nil {
				logger.WarnContext(ctx, "outbox completion rejected", "event_id", delivery.EventID, "handler", name, "error", err)
				continue
			}
			logger.InfoContext(ctx, "outbox delivery completed", "event_id", delivery.EventID, "event_type", delivery.EventType, "handler", name)
		}
		if !worked {
			if wait(ctx, 500*time.Millisecond) != nil {
				return nil
			}
		}
	}
	return nil
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
