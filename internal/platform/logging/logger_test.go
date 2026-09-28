package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewUsesConfiguredLevel(t *testing.T) {
	logger := New("production", slog.LevelWarn)
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("error should be enabled at warn level")
	}
}

func TestNewSelectsDevelopmentDebugLevelFromConfiguration(t *testing.T) {
	logger := New("development", slog.LevelDebug)
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("configured debug level should be enabled")
	}
}
