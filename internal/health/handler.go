package health

import (
	"log/slog"
	"net/http"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/response"
)

type Handler struct {
	logger       *slog.Logger
	errorManager *errormanager.Manager
}

func NewHandler(logger *slog.Logger, errors *errormanager.Manager) *Handler {
	return &Handler{
		logger:       logger,
		errorManager: errors,
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
