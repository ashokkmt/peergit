package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"peergit/internal/campus"
	"peergit/internal/github"
	"peergit/internal/health"
	"peergit/internal/identity"
	"peergit/internal/media"
	"peergit/internal/platform/config"
	"peergit/internal/platform/database"
	"peergit/internal/platform/errormanager"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/logging"
	"peergit/internal/platform/storage"
	"peergit/internal/project"
	"peergit/internal/recruitment"
	"peergit/internal/repository"
)

func main() {
	if err := run(); err != nil {
		slog.Error("API server stopped", "error", err)
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
	errorManager := errormanager.NewManager(logger)
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, err = database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMax)
		cancel()
		if err != nil {
			return err
		}
		defer pool.Close()
	}
	healthHandler := health.NewHandler(logger, errorManager, pool)
	identityHandler := identity.NewHandler(pool, identity.Config{
		GitHubClientID: cfg.GitHubClientID, GitHubClientSecret: cfg.GitHubClientSecret, GitHubRedirectURL: cfg.GitHubRedirectURL,
		GitHubAuthorizeURL: cfg.GitHubAuthorizeURL, GitHubTokenURL: cfg.GitHubTokenURL, GitHubAPIURL: cfg.GitHubAPIURL,
		AppOrigin: cfg.AppOrigin, CookieSecure: cfg.CookieSecure,
		SessionHashKey: cfg.SessionHashKey, MFAEncryptionKey: cfg.MFAEncryptionKey,
	}, logger, errorManager)
	campusHandler := campus.NewHandler(pool, identityHandler, cfg.AppOrigin, errorManager, cfg.VerificationHashKey, cfg.VerificationEmailKey)
	var objectStore *storage.Store
	if cfg.ObjectEndpoint != "" {
		objectStore = storage.New(cfg.ObjectEndpoint, cfg.ObjectBucket, cfg.ObjectRegion, cfg.ObjectAccessKey, cfg.ObjectSecretKey)
	}
	mediaHandler := media.NewHandler(pool, objectStore, identityHandler, errorManager)
	projectHandler := project.NewHandler(pool, identityHandler, logger, errorManager)
	recruitmentHandler := recruitment.NewHandler(pool, identityHandler, logger, errorManager)
	githubClient := github.New()
	if cfg.GitHubAPIURL != "" {
		githubClient.API = cfg.GitHubAPIURL
	}
	var githubKey []byte
	if cfg.GitHubAppPrivateKey != "" {
		githubKey, err = os.ReadFile(cfg.GitHubAppPrivateKey)
		if err != nil {
			return fmt.Errorf("read GitHub App private key: %w", err)
		}
	}
	repositoryHandler := repository.NewHandler(pool, identityHandler, githubClient, objectStore, repository.AppConfig{
		ID: cfg.GitHubAppID, Slug: cfg.GitHubAppSlug, PrivateKey: githubKey, WebhookSecret: cfg.GitHubWebhookSecret, Origin: cfg.AppOrigin,
		OAuthClientID: cfg.GitHubRepositoryClientID, OAuthClientSecret: cfg.GitHubRepositoryClientSecret, OAuthRedirectURL: cfg.GitHubRepositoryRedirectURL,
		OAuthAuthorizeURL: cfg.GitHubAuthorizeURL, OAuthTokenURL: cfg.GitHubTokenURL, APIURL: cfg.GitHubAPIURL,
	}, logger, errorManager)
	router := httpserver.NewRouter(healthHandler, logger, errorManager, identityHandler.Register, campusHandler.Register, mediaHandler.Register, projectHandler.Register, recruitmentHandler.Register, repositoryHandler.Register)

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	defer listener.Close()

	server := newHTTPServer(router)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("API server starting",
		"addr", listener.Addr().String(),
	)
	return serve(ctx, server, listener)
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// SSE responses are long-lived, so WriteTimeout stays unset.
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()

	select {
	case err := <-serveResult:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		err := <-serveResult
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
