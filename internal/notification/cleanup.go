package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunCleanup(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	clean := func() {
		if _, err := pool.Exec(ctx, `SELECT cleanup_notifications()`); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "notification retention cleanup failed", "error", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM notification_stream_leases WHERE expires_at<=clock_timestamp()`); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "notification stream lease cleanup failed", "error", err)
		}
	}
	clean()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clean()
		}
	}
}
