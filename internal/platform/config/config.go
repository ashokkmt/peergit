package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv   string
	HTTPAddr string
	LogLevel slog.Level
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}
	appEnv := getEnv("APP_ENV", "development")
	defaultLogLevel := "info"
	if appEnv == "development" {
		defaultLogLevel = "debug"
	}
	logLevel, err := parseLogLevel(getEnv("LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		AppEnv:   appEnv,
		HTTPAddr: getEnv("HTTP_ADDR", ":8080"),
		LogLevel: logLevel,
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	switch c.AppEnv {
	case "development", "test", "staging", "production":
	default:
		return fmt.Errorf("APP_ENV must be development, test, staging, or production")
	}
	if c.HTTPAddr == "" {
		return errors.New("HTTP_ADDR is required")
	}
	_, portText, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR must be host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("HTTP_ADDR port must be between 1 and 65535")
	}
	return nil
}

func parseLogLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(value))); err != nil {
		return 0, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error: %w", err)
	}
	return level, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
