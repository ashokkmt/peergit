package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv             string
	HTTPAddr           string
	DatabaseURL        string
	DatabaseMax        int32
	CursorKey          string
	AppOrigin          string
	CookieSecure       bool
	SessionHashKey     string
	MFAEncryptionKey   string
	GoogleIssuer       string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	ObjectEndpoint     string
	ObjectRegion       string
	ObjectBucket       string
	ObjectAccessKey    string
	ObjectSecretKey    string
	LogLevel           slog.Level
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

	databaseMax, err := strconv.ParseInt(getEnv("DATABASE_MAX_CONNS", "20"), 10, 32)
	if err != nil || databaseMax < 1 || databaseMax > 200 {
		return nil, errors.New("DATABASE_MAX_CONNS must be between 1 and 200")
	}
	cookieSecure, err := strconv.ParseBool(getEnv("COOKIE_SECURE", "true"))
	if err != nil {
		return nil, errors.New("COOKIE_SECURE must be true or false")
	}
	cfg := &Config{
		AppEnv:             appEnv,
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		DatabaseMax:        int32(databaseMax),
		CursorKey:          getEnv("CURSOR_SIGNING_KEY", ""),
		AppOrigin:          getEnv("APP_ORIGIN", "https://localhost"),
		CookieSecure:       cookieSecure,
		SessionHashKey:     getEnv("SESSION_HASH_KEY", "local-development-only-session-hash-key"),
		MFAEncryptionKey:   getEnv("MFA_ENCRYPTION_KEY", "local-development-only-mfa-encryption-key"),
		GoogleIssuer:       getEnv("GOOGLE_OIDC_ISSUER", "https://accounts.google.com"),
		GoogleClientID:     getEnv("GOOGLE_OIDC_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_OIDC_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_OIDC_REDIRECT_URL", ""),
		ObjectEndpoint:     getEnv("OBJECT_ENDPOINT", ""),
		ObjectRegion:       getEnv("OBJECT_REGION", "auto"),
		ObjectBucket:       getEnv("OBJECT_BUCKET", ""),
		ObjectAccessKey:    getEnv("OBJECT_ACCESS_KEY", ""),
		ObjectSecretKey:    getEnv("OBJECT_SECRET_KEY", ""),
		LogLevel:           logLevel,
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
	if c.DatabaseURL != "" {
		u, err := url.Parse(c.DatabaseURL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || u.Path == "/" || u.Path == "" {
			return errors.New("DATABASE_URL must be a PostgreSQL URL")
		}
	}
	if c.AppEnv == "production" || c.AppEnv == "staging" {
		if c.DatabaseURL == "" {
			return errors.New("DATABASE_URL is required in staging and production")
		}
		if len(c.CursorKey) < 32 {
			return errors.New("CURSOR_SIGNING_KEY must be at least 32 bytes in staging and production")
		}
		if c.CursorKey == "local-development-only-cursor-key-change-before-deploy" {
			return errors.New("CURSOR_SIGNING_KEY must not use the example development value")
		}
		if !c.CookieSecure || len(c.SessionHashKey) < 32 || len(c.MFAEncryptionKey) < 32 ||
			strings.HasPrefix(c.SessionHashKey, "local-development-only") || strings.HasPrefix(c.MFAEncryptionKey, "local-development-only") {
			return errors.New("staging and production require secure cookies and unique session/MFA keys of at least 32 bytes")
		}
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" || c.GoogleRedirectURL == "" {
			return errors.New("google OIDC client ID, client secret, and redirect URL are required in staging and production")
		}
	}
	origin, err := url.Parse(c.AppOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || (origin.Scheme != "https" && c.AppEnv != "development" && c.AppEnv != "test") {
		return errors.New("APP_ORIGIN must be an origin URL without path, query, or fragment")
	}
	if (c.GoogleClientID == "") != (c.GoogleClientSecret == "") || (c.GoogleClientID == "") != (c.GoogleRedirectURL == "") {
		return errors.New("google OIDC client ID, secret, and redirect URL must be configured together")
	}
	if c.GoogleClientID != "" {
		redirect, err := url.Parse(c.GoogleRedirectURL)
		if err != nil || redirect.Scheme != origin.Scheme || redirect.Host != origin.Host || redirect.Path != "/api/v1/auth/callback" || redirect.RawQuery != "" || redirect.Fragment != "" {
			return errors.New("GOOGLE_OIDC_REDIRECT_URL must use APP_ORIGIN and /api/v1/auth/callback")
		}
		if c.AppEnv != "development" && c.AppEnv != "test" && c.GoogleIssuer != "https://accounts.google.com" {
			return errors.New("GOOGLE_OIDC_ISSUER must be https://accounts.google.com outside development and test")
		}
	}
	objectParts := 0
	for _, value := range []string{c.ObjectEndpoint, c.ObjectBucket, c.ObjectAccessKey, c.ObjectSecretKey} {
		if value != "" {
			objectParts++
		}
	}
	if objectParts != 0 && objectParts != 4 {
		return errors.New("object storage endpoint, bucket, access key, and secret must be configured together")
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
