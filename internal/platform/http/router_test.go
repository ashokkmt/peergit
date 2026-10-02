package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"peergit/internal/health"
	"peergit/internal/platform/errormanager"
	"peergit/internal/platform/http/response"
)

func TestRouterAddsRequestIDAndLogsRequest(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	manager := errormanager.NewManager(logger)
	router := NewRouter(health.NewHandler(logger, manager, nil), logger, manager)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	id := w.Header().Get("X-Request-ID")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		t.Fatalf("invalid generated request ID %q", id)
	}
	var got response.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RequestID != id || w.Code != http.StatusOK {
		t.Fatalf("request ID/status mismatch: %#v, %d", got, w.Code)
	}
	if !strings.Contains(logs.String(), id) || !strings.Contains(logs.String(), "http request completed") {
		t.Fatal("access log missing request ID or completion event")
	}
}

func TestRecoverPanicsReturnsGenericError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	manager := errormanager.NewManager(logger)
	router := &Router{logger: logger, errorManager: manager}
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private detail") })
	handler := router.accessLog(router.recoverPanics(panicHandler))
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "test-id")
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "internal_error") || strings.Contains(w.Body.String(), "private detail") {
		t.Fatalf("unexpected recovered response: %d %s", w.Code, w.Body.String())
	}
}

func TestRecoverPanicsDoesNotAppendAfterResponseCommit(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	manager := errormanager.NewManager(logger)
	router := &Router{logger: logger, errorManager: manager}
	handler := router.accessLog(router.recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("started"))
		panic("late failure")
	})))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)
	r.Header.Set("X-Request-ID", "test-id")
	func() {
		defer func() {
			if got := recover(); got != http.ErrAbortHandler {
				t.Fatalf("panic = %v, want ErrAbortHandler", got)
			}
		}()
		handler.ServeHTTP(w, r)
	}()
	if w.Code != http.StatusAccepted || w.Body.String() != "started" {
		t.Fatalf("panic corrupted committed response: %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "request failed after response started") {
		t.Fatal("expected post-commit panic to be logged")
	}
	if !strings.Contains(logs.String(), "test-id") {
		t.Fatal("post-commit panic log missing request ID")
	}
}

func TestRecoverPanicsPreservesAbortHandler(t *testing.T) {
	manager := errormanager.NewManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := &Router{errorManager: manager}
	handler := router.recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if got := recover(); got != http.ErrAbortHandler {
			t.Fatalf("panic = %v, want ErrAbortHandler", got)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestStreamingPanicAbortsNetworkResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := errormanager.NewManager(logger)
	router := &Router{logger: logger, errorManager: manager}
	handler := router.accessLog(router.recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("started"))
		w.(http.Flusher).Flush()
		panic("stream failed")
	})))
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if string(body) != "started" || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("body = %q, error = %v; want started and unexpected EOF", body, err)
	}
}

func TestRouterFallbacksUseErrorEnvelope(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := errormanager.NewManager(logger)
	router := NewRouter(health.NewHandler(logger, manager, nil), logger, manager)
	for _, tt := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/missing", "route_not_found", http.StatusNotFound},
		{http.MethodPost, "/healthz", "method_not_allowed", http.StatusMethodNotAllowed},
	} {
		t.Run(tt.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			var got response.Envelope
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != tt.status || got.Error == nil || got.Error.Code != tt.code || got.RequestID == "" || got.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatalf("unexpected error response: %d %#v", w.Code, got)
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Fatal("error response must be JSON")
			}
		})
	}
}
