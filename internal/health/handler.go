package health

import (
	"log/slog"
	"net/http"
	"peergit/internal/platform/http/response"
)

type Handler struct {
	logger *slog.Logger
}

func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{
		logger: logger,
	}
}

func (h Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("health check requested")

	response.OK(w, Response{
		Status: "ok",
	})
}
