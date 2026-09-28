package errormanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peergit/internal/platform/http/response"
)

func TestHandleMapsKnownErrorAndKeepsCausePrivate(t *testing.T) {
	var logs bytes.Buffer
	manager := NewManager(slog.New(slog.NewTextHandler(&logs, nil)))
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "request-7")
	r := httptest.NewRequest("GET", "/", nil)
	manager.Handle(w, r, BadRequest("bad_input", "input is invalid", errors.New("private database detail")))

	var got response.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusBadRequest || got.Error == nil || got.Error.Code != "bad_input" || got.RequestID != "request-7" {
		t.Fatalf("unexpected response: %d %#v", w.Code, got)
	}
	if strings.Contains(w.Body.String(), "private database detail") {
		t.Fatal("internal cause leaked to client")
	}
	if !strings.Contains(logs.String(), "bad_input") {
		t.Fatal("expected client rejection to be logged")
	}
}

func TestHandleConvertsUnknownErrorToGenericInternalError(t *testing.T) {
	manager := NewManager(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	w := httptest.NewRecorder()
	manager.Handle(w, httptest.NewRequest("GET", "/", nil), New(http.StatusInternalServerError, "database_error", "secret detail", errors.New("connection password")))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret detail") || !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatalf("unexpected generic error response: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "connection password") {
		t.Fatal("internal cause leaked to client")
	}
}

func TestHandleDoesNotWriteAfterResponseWasCommitted(t *testing.T) {
	manager := NewManager(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	w := &committedWriter{ResponseRecorder: httptest.NewRecorder(), status: http.StatusOK}
	manager.Handle(w, httptest.NewRequest("GET", "/", nil), errors.New("late error"))
	if w.Body.Len() != 0 {
		t.Fatalf("wrote an error body after commit: %q", w.Body.String())
	}
}

type committedWriter struct {
	*httptest.ResponseRecorder
	status int
}

func (w *committedWriter) Status() int { return w.status }
