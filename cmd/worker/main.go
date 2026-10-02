package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"peergit/internal/platform/config"
	"peergit/internal/platform/database"
	"peergit/internal/platform/idempotency"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/logging"
	"peergit/internal/platform/outbox"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.AppEnv, cfg.LogLevel)
	slog.SetDefault(logger)
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required for the worker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMax)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerName, err := os.Hostname()
	if err != nil {
		return errors.New("cannot determine worker name")
	}
	workerName += "-" + strconv.Itoa(os.Getpid())
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	results := make(chan error, 3)
	go func() {
		results <- (jobs.Queue{Pool: pool}).Run(workerCtx, workerName, map[string]jobs.Handler{}, logger)
	}()
	go func() {
		results <- (outbox.Queue{Pool: pool}).Run(workerCtx, workerName, map[string]outbox.Handler{}, logger)
	}()
	go func() {
		idempotency.RunCleanup(workerCtx, pool, logger)
		results <- nil
	}()
	for range 3 {
		if err := <-results; err != nil {
			cancelWorker()
			return err
		}
	}
	return nil
}
