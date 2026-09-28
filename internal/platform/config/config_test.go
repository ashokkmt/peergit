package config

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	for input, want := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	} {
		got, err := parseLogLevel(input)
		if err != nil || got != want {
			t.Errorf("parseLogLevel(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	if _, err := parseLogLevel("verbose"); err == nil {
		t.Fatal("unknown log level should fail")
	}
}

func TestLoadDefaultsAndHonorsLogLevel(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("LOG_LEVEL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("development default log level = %v, want debug", cfg.LogLevel)
	}
	t.Setenv("LOG_LEVEL", "ERROR")
	cfg, err = Load()
	if err != nil || cfg.LogLevel != slog.LevelError {
		t.Fatalf("configured log level = %v, %v; want error", cfg.LogLevel, err)
	}
	t.Setenv("LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("invalid log level should fail startup configuration")
	}
}

func TestConfigValidation(t *testing.T) {
	valid := Config{AppEnv: "production", HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, cfg := range []Config{
		{AppEnv: "prod", HTTPAddr: ":8080"},
		{AppEnv: "production", HTTPAddr: "no-port"},
		{AppEnv: "production", HTTPAddr: ":70000"},
	} {
		if err := cfg.Validate(); err == nil {
			t.Errorf("invalid config accepted: %#v", cfg)
		}
	}
}
