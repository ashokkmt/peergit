package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/response"
)

type Handler struct {
	logger       *slog.Logger
	errorManager *errormanager.Manager
	pool         *pgxpool.Pool
}

func NewHandler(logger *slog.Logger, errors *errormanager.Manager, pool *pgxpool.Pool) *Handler {
	return &Handler{
		logger:       logger,
		errorManager: errors,
		pool:         pool,
	}
}

func (h Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		h.errorManager.Handle(w, r, errormanager.New(http.StatusServiceUnavailable, "dependency_unavailable", "service dependencies are unavailable", nil))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		h.errorManager.Handle(w, r, errormanager.New(http.StatusServiceUnavailable, "dependency_unavailable", "service dependencies are unavailable", err))
		return
	}
	if err := response.OK(w, Response{Status: "ready"}); err != nil {
		h.errorManager.Handle(w, r, err)
	}
}

func (h Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("health check requested", "request_id", r.Header.Get("X-Request-ID"))

	if err := response.OK(w, Response{
		Status: "ok",
	}); err != nil {
		h.errorManager.Handle(w, r, err)
	}
}
