package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                       string
	HTTPAddr                     string
	DatabaseURL                  string
	DatabaseMax                  int32
	CursorKey                    string
	AppOrigin                    string
	CookieSecure                 bool
	SessionHashKey               string
	MFAEncryptionKey             string
	GitHubClientID               string
	GitHubClientSecret           string
	GitHubRedirectURL            string
	GitHubAuthorizeURL           string
	GitHubTokenURL               string
	GitHubAPIURL                 string
	GitHubAppID                  string
	GitHubAppSlug                string
	GitHubAppPrivateKey          string
	GitHubWebhookSecret          string
	GitHubRepositoryClientID     string
	GitHubRepositoryClientSecret string
	GitHubRepositoryRedirectURL  string
	SMTPHost                     string
	SMTPFrom                     string
	SMTPUsername                 string
	SMTPPassword                 string
	SMTPTLSMode                  string
	VerificationHashKey          string
	VerificationEmailKey         string
	ObjectEndpoint               string
	ObjectRegion                 string
	ObjectBucket                 string
	ObjectAccessKey              string
	ObjectSecretKey              string
	LogLevel                     slog.Level
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
		AppEnv:                       appEnv,
		HTTPAddr:                     getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:                  getEnv("DATABASE_URL", ""),
		DatabaseMax:                  int32(databaseMax),
		CursorKey:                    getEnv("CURSOR_SIGNING_KEY", ""),
		AppOrigin:                    getEnv("APP_ORIGIN", "https://localhost"),
		CookieSecure:                 cookieSecure,
		SessionHashKey:               getEnv("SESSION_HASH_KEY", "local-development-only-session-hash-key"),
		MFAEncryptionKey:             getEnv("MFA_ENCRYPTION_KEY", "local-development-only-mfa-encryption-key"),
		GitHubClientID:               getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:           getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURL:            getEnv("GITHUB_REDIRECT_URL", ""),
		GitHubAuthorizeURL:           getEnv("PEERGIT_GITHUB_AUTHORIZE_URL", ""),
		GitHubTokenURL:               getEnv("PEERGIT_GITHUB_TOKEN_URL", ""),
		GitHubAPIURL:                 getEnv("PEERGIT_GITHUB_API_URL", ""),
		GitHubAppID:                  getEnv("GITHUB_APP_ID", ""),
		GitHubAppSlug:                getEnv("GITHUB_APP_SLUG", ""),
		GitHubAppPrivateKey:          getEnv("GITHUB_APP_PRIVATE_KEY_PATH", ""),
		GitHubWebhookSecret:          getEnv("GITHUB_APP_WEBHOOK_SECRET", ""),
		GitHubRepositoryClientID:     getEnv("GITHUB_REPOSITORY_CLIENT_ID", ""),
		GitHubRepositoryClientSecret: getEnv("GITHUB_REPOSITORY_CLIENT_SECRET", ""),
		GitHubRepositoryRedirectURL:  getEnv("GITHUB_REPOSITORY_REDIRECT_URL", ""),
		SMTPHost:                     getEnv("SMTP_HOST", "127.0.0.1:1025"),
		SMTPFrom:                     getEnv("SMTP_FROM", "PeerGit <noreply@localhost>"),
		SMTPUsername:                 getEnv("SMTP_USERNAME", ""),
		SMTPPassword:                 getEnv("SMTP_PASSWORD", ""),
		SMTPTLSMode:                  getEnv("SMTP_TLS_MODE", "none"),
		VerificationHashKey:          getEnv("CAMPUS_VERIFICATION_HASH_KEY", "local-development-only-verification-hash-key"),
		VerificationEmailKey:         getEnv("VERIFICATION_EMAIL_ENCRYPTION_KEY", "local-development-only-verification-encryption-key"),
		ObjectEndpoint:               getEnv("OBJECT_ENDPOINT", ""),
		ObjectRegion:                 getEnv("OBJECT_REGION", "auto"),
		ObjectBucket:                 getEnv("OBJECT_BUCKET", ""),
		ObjectAccessKey:              getEnv("OBJECT_ACCESS_KEY", ""),
		ObjectSecretKey:              getEnv("OBJECT_SECRET_KEY", ""),
		LogLevel:                     logLevel,
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
		if c.GitHubClientID == "" || c.GitHubClientSecret == "" || c.GitHubRedirectURL == "" {
			return errors.New("GitHub client ID, client secret, and redirect URL are required in staging and production")
		}
		if c.GitHubAppID == "" || c.GitHubAppSlug == "" || c.GitHubAppPrivateKey == "" || len(c.GitHubWebhookSecret) < 32 {
			return errors.New("GitHub App ID, slug, private-key path, and webhook secret are required in staging and production")
		}
		if c.GitHubRepositoryClientID == "" || c.GitHubRepositoryClientSecret == "" || c.GitHubRepositoryRedirectURL == "" {
			return errors.New("repository GitHub App client ID, secret, and redirect URL are required in staging and production")
		}
	}
	origin, err := url.Parse(c.AppOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || (origin.Scheme != "https" && c.AppEnv != "development" && c.AppEnv != "test") {
		return errors.New("APP_ORIGIN must be an origin URL without path, query, or fragment")
	}
	if (c.GitHubClientID == "") != (c.GitHubClientSecret == "") || (c.GitHubClientID == "") != (c.GitHubRedirectURL == "") {
		return errors.New("GitHub client ID, secret, and redirect URL must be configured together")
	}
	if (c.GitHubRepositoryClientID == "") != (c.GitHubRepositoryClientSecret == "") || (c.GitHubRepositoryClientID == "") != (c.GitHubRepositoryRedirectURL == "") {
		return errors.New("repository GitHub App client ID, secret, and redirect URL must be configured together")
	}
	appParts := 0
	for _, value := range []string{c.GitHubAppID, c.GitHubAppSlug, c.GitHubAppPrivateKey, c.GitHubWebhookSecret} {
		if value != "" {
			appParts++
		}
	}
	if appParts != 0 && appParts != 4 {
		return errors.New("GitHub App ID, slug, private-key path, and webhook secret must be configured together")
	}
	if c.GitHubAppID != "" {
		if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(c.GitHubAppID) || !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(c.GitHubAppSlug) || len(c.GitHubWebhookSecret) < 32 {
			return errors.New("GitHub App ID, slug, or webhook secret is invalid")
		}
	}
	if c.GitHubClientID != "" {
		redirect, err := url.Parse(c.GitHubRedirectURL)
		if err != nil || redirect.Scheme != origin.Scheme || redirect.Host != origin.Host || redirect.Path != "/api/v1/auth/github/callback" || redirect.RawQuery != "" || redirect.Fragment != "" {
			return errors.New("GITHUB_REDIRECT_URL must use APP_ORIGIN and /api/v1/auth/github/callback")
		}
	}
	if c.GitHubRepositoryClientID != "" {
		redirect, err := url.Parse(c.GitHubRepositoryRedirectURL)
		if err != nil || redirect.Scheme != origin.Scheme || redirect.Host != origin.Host || redirect.Path != "/api/v1/github/authorization/callback" || redirect.RawQuery != "" || redirect.Fragment != "" {
			return errors.New("GITHUB_REPOSITORY_REDIRECT_URL must use APP_ORIGIN and /api/v1/github/authorization/callback")
		}
	}
	if c.SMTPTLSMode != "none" && c.SMTPTLSMode != "starttls" && c.SMTPTLSMode != "tls" {
		return errors.New("SMTP_TLS_MODE must be none, starttls, or tls")
	}
	if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
		return errors.New("SMTP_USERNAME and SMTP_PASSWORD must be configured together")
	}
	if c.AppEnv == "production" || c.AppEnv == "staging" {
		if c.SMTPTLSMode == "none" || c.SMTPFrom == "" || len(c.VerificationHashKey) < 32 || len(c.VerificationEmailKey) < 32 || strings.HasPrefix(c.VerificationHashKey, "local-development-only") || strings.HasPrefix(c.VerificationEmailKey, "local-development-only") {
			return errors.New("staging and production require authenticated mail configuration and unique verification keys")
		}
	}
	for _, endpoint := range []string{c.GitHubAuthorizeURL, c.GitHubTokenURL, c.GitHubAPIURL} {
		if endpoint != "" && c.AppEnv != "test" {
			return errors.New("GitHub endpoint overrides are allowed only in APP_ENV=test")
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
