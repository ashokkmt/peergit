package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"peergit/internal/platform/config"
	"peergit/internal/platform/database"
	"peergit/internal/platform/logging"
	"peergit/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migration command failed", "error", err)
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
		return errors.New("DATABASE_URL is required to run migrations")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMax)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool, migrations.Files); err != nil {
		return err
	}
	logger.Info("database migrations applied")
	return nil
}
