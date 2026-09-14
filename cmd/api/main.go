package main

import (
	"net/http"
	"peergit/internal/health"
	httpserver "peergit/internal/platform/http"
	"peergit/internal/platform/logging"
	"peergit/internal/platform/config"
)

func main() {
	cfg := config.Load()
	logger := logging.New(cfg.AppEnv)
	healthHandler := health.NewHandler(logger)

	router := httpserver.NewRouter(healthHandler)

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	logger.Info("API server starting",
		"addr", server.Addr,
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("API server stopped unexpectedly",
			"error", err,
		)
	}
}
