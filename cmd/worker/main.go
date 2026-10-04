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

	"peergit/internal/campus"
	"peergit/internal/github"
	"peergit/internal/platform/config"
	"peergit/internal/platform/database"
	"peergit/internal/platform/idempotency"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/logging"
	"peergit/internal/platform/outbox"
	"peergit/internal/platform/storage"
	"peergit/internal/repository"
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
	results := make(chan error, 5)
	githubClient := github.New()
	if cfg.GitHubAPIURL != "" {
		githubClient.API = cfg.GitHubAPIURL
	}
	var githubKey []byte
	if cfg.GitHubAppPrivateKey != "" {
		githubKey, err = os.ReadFile(cfg.GitHubAppPrivateKey)
		if err != nil {
			return err
		}
	}
	var objectStore *storage.Store
	if cfg.ObjectEndpoint != "" {
		objectStore = storage.New(cfg.ObjectEndpoint, cfg.ObjectBucket, cfg.ObjectRegion, cfg.ObjectAccessKey, cfg.ObjectSecretKey)
	}
	go func() {
		handlers := map[string]jobs.Handler{
			"campus_verification_email": campus.DeliveryHandler(pool, cfg.VerificationEmailKey, campus.MailConfig{Host: cfg.SMTPHost, From: cfg.SMTPFrom, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, TLSMode: cfg.SMTPTLSMode}),
		}
		for jobType, handler := range repository.JobHandlers(pool, githubClient, cfg.GitHubAppID, githubKey, objectStore, logger) {
			handlers[jobType] = handler
		}
		results <- (jobs.Queue{Pool: pool}).RunConcurrent(workerCtx, workerName, handlers, logger, 10)
	}()
	go func() {
		results <- (outbox.Queue{Pool: pool}).Run(workerCtx, workerName, map[string]outbox.Handler{}, logger)
	}()
	go func() { campus.RunDeliveryCleanup(workerCtx, pool, logger); results <- nil }()
	go func() {
		idempotency.RunCleanup(workerCtx, pool, logger)
		results <- nil
	}()
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for workerCtx.Err() == nil {
			if err := repository.ScheduleReconciliation(workerCtx, pool, logger); err != nil {
				logger.ErrorContext(workerCtx, "repository reconciliation schedule failed", "error", err)
			}
			select {
			case <-workerCtx.Done():
				results <- nil
				return
			case <-ticker.C:
			}
		}
		results <- nil
	}()
	for range 5 {
		if err := <-results; err != nil {
			cancelWorker()
			return err
		}
	}
	return nil
}
