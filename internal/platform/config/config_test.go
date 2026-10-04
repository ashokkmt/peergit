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
	valid := Config{
		AppEnv: "production", HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo,
		DatabaseURL: "postgres://user:pass@localhost/peergit", CursorKey: "0123456789abcdef0123456789abcdef",
		AppOrigin: "https://example.edu", CookieSecure: true,
		SessionHashKey: "0123456789abcdef0123456789abcdef", MFAEncryptionKey: "abcdef0123456789abcdef0123456789",
		GitHubClientID: "client-id", GitHubClientSecret: "client-secret", GitHubRedirectURL: "https://example.edu/api/v1/auth/github/callback",
		SMTPHost: "smtp.example.edu:587", SMTPFrom: "PeerGit <notify@example.edu>", SMTPTLSMode: "starttls", VerificationHashKey: "unique-verification-hash-key-for-test", VerificationEmailKey: "unique-verification-encryption-key-test",
	}
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
	if err := (Config{AppEnv: "staging", HTTPAddr: ":8080"}).Validate(); err == nil {
		t.Fatal("staging must require production-shaped database and cursor configuration")
	}
	valid.CursorKey = "local-development-only-cursor-key-change-before-deploy"
	if err := valid.Validate(); err == nil {
		t.Fatal("production must reject the example development signing key")
	}
	valid.CursorKey = "0123456789abcdef0123456789abcdef"
	valid.GitHubRedirectURL = "https://attacker.example/api/v1/auth/github/callback"
	if err := valid.Validate(); err == nil {
		t.Fatal("GitHub callback on another origin was accepted")
	}
	dev := Config{AppEnv: "development", HTTPAddr: "127.0.0.1:8080", AppOrigin: "https://localhost", LogLevel: slog.LevelInfo}
	dev.GitHubClientID = "partial-client"
	if err := dev.Validate(); err == nil {
		t.Fatal("partial GitHub App configuration was accepted")
	}
	dev.GitHubClientID = ""
	dev.ObjectEndpoint = "http://127.0.0.1:8333"
	if err := dev.Validate(); err == nil {
		t.Fatal("partial object storage configuration was accepted")
	}
}
