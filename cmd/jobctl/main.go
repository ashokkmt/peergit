package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"peergit/internal/platform/config"
	"peergit/internal/platform/database"
	"peergit/internal/platform/jobs"
	"peergit/internal/platform/logging"
	"peergit/internal/platform/outbox"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("job operator command failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: go run ./cmd/jobctl list-dead [limit] | requeue-dead <job-id> | list-outbox-dead [limit] | requeue-outbox-dead <event-id> <handler>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.AppEnv, cfg.LogLevel)
	slog.SetDefault(logger)
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMax)
	if err != nil {
		return err
	}
	defer pool.Close()
	queue := jobs.Queue{Pool: pool}
	deliveries := outbox.Queue{Pool: pool}
	switch args[0] {
	case "list-dead":
		limit := 25
		if len(args) > 2 {
			return errors.New("usage: go run ./cmd/jobctl list-dead [limit]")
		}
		if len(args) == 2 {
			limit, err = strconv.Atoi(args[1])
			if err != nil || limit < 1 || limit > 100 {
				return errors.New("limit must be between 1 and 100")
			}
		}
		dead, err := queue.ListDead(ctx, limit)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(dead)
	case "requeue-dead":
		if len(args) != 2 {
			return errors.New("usage: go run ./cmd/jobctl requeue-dead <job-id>")
		}
		if err := queue.RequeueDead(ctx, args[1]); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "job queued for another attempt")
		return nil
	case "list-outbox-dead":
		limit := 25
		if len(args) > 2 {
			return errors.New("usage: go run ./cmd/jobctl list-outbox-dead [limit]")
		}
		if len(args) == 2 {
			limit, err = strconv.Atoi(args[1])
			if err != nil || limit < 1 || limit > 100 {
				return errors.New("limit must be between 1 and 100")
			}
		}
		dead, err := deliveries.ListDead(ctx, limit)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(dead)
	case "requeue-outbox-dead":
		if len(args) != 3 {
			return errors.New("usage: go run ./cmd/jobctl requeue-outbox-dead <event-id> <handler>")
		}
		if err := deliveries.RequeueDead(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "outbox delivery queued for another attempt")
		return nil
	default:
		return errors.New("unknown command; use list-dead or requeue-dead")
	}
}
