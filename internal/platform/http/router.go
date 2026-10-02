package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"peergit/internal/health"
	"peergit/internal/platform/errormanager"
)

type Router struct {
	logger       *slog.Logger
	errorManager *errormanager.Manager
	requests     atomic.Uint64
	errors       atomic.Uint64
}

func NewRouter(healthHandler *health.Handler, logger *slog.Logger, errorManager *errormanager.Manager) http.Handler {
	router := &Router{logger: logger, errorManager: errorManager}
	r := chi.NewRouter()
	r.Use(router.requestID)
	r.Use(router.accessLog)
	r.Use(router.recoverPanics)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		router.errorManager.Handle(w, r, errormanager.New(http.StatusNotFound, "route_not_found", "route not found", nil))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		router.errorManager.Handle(w, r, errormanager.New(http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil))
	})

	r.Get("/healthz", healthHandler.Healthz)
	r.Get("/readyz", healthHandler.Readyz)
	r.Get("/metrics", router.metrics)
	return r
}

func (router *Router) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			router.errorManager.Handle(w, r, err)
			return
		}
		requestID := hex.EncodeToString(id[:])
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func (router *Router) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			router.requests.Add(1)
			if status >= http.StatusInternalServerError {
				router.errors.Add(1)
			}
			router.logger.InfoContext(r.Context(), "http request completed",
				"request_id", w.Header().Get("X-Request-ID"),
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", wrapped.BytesWritten(),
				"duration", time.Since(started),
			)
		}()
		next.ServeHTTP(wrapped, r)
	})
}

func (router *Router) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = fmt.Fprintf(w, "peergit_http_requests_total %d\npeergit_http_errors_total %d\n", router.requests.Load(), router.errors.Load())
}

func (router *Router) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				err := fmt.Errorf("handler panic: %v", value)
				if committed, ok := w.(interface{ Status() int }); ok && committed.Status() != 0 {
					router.errorManager.Report(r, err)
					panic(http.ErrAbortHandler)
				}
				router.errorManager.Handle(w, r, err)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
