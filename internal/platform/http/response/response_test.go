package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOKWritesEnvelopeAndRequestID(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "abc123")
	if err := OK(w, map[string]string{"status": "ok"}); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected response status/header: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	var got Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RequestID != "abc123" || got.Error != nil {
		t.Fatalf("unexpected envelope: %#v", got)
	}
}

func TestJSONMarshalFailureDoesNotCommitResponse(t *testing.T) {
	w := httptest.NewRecorder()
	if err := JSON(w, http.StatusOK, make(chan int)); err == nil {
		t.Fatal("expected unsupported value to fail marshaling")
	}
	if w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
		t.Fatalf("marshal failure committed response: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestJSONRejectsInvalidStatusBeforeWriting(t *testing.T) {
	w := httptest.NewRecorder()
	if err := JSON(w, 42, map[string]string{"ok": "yes"}); err == nil {
		t.Fatal("invalid HTTP status should fail")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("invalid status wrote a body: %q", w.Body.String())
	}
}

func TestWriteErrorUsesStableEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req-1")
	if err := WriteError(w, http.StatusBadRequest, "invalid_input", "request is invalid"); err != nil {
		t.Fatal(err)
	}
	var got Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusBadRequest || got.Error == nil || got.Error.Code != "invalid_input" || got.RequestID != "req-1" {
		t.Fatalf("unexpected error response: status=%d envelope=%#v", w.Code, got)
	}
}

func TestNoContentWritesNoBody(t *testing.T) {
	w := httptest.NewRecorder()
	NoContent(w)
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("unexpected no-content response: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestVersionSetsStrongETagAndRejectsInvalidVersion(t *testing.T) {
	w := httptest.NewRecorder()
	if err := Version(w, 7); err != nil || w.Header().Get("ETag") != `"v7"` {
		t.Fatalf("ETag=%q err=%v", w.Header().Get("ETag"), err)
	}
	if err := Version(w, 0); err == nil {
		t.Fatal("non-positive version accepted")
	}
}
