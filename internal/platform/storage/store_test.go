package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCaptureVerifiesBytesAndUsesUniqueKeys(t *testing.T) {
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if _, ok := objects[r.URL.Path]; ok {
				w.WriteHeader(412)
				return
			}
			objects[r.URL.Path] = data
			w.Header().Set("ETag", `"fixture"`)
		case "GET":
			_, _ = w.Write(objects[r.URL.Path])
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	s := New(server.URL, "bucket", "us-east-1", "fixture", "fixture")
	first, err := s.Capture(context.Background(), strings.NewReader("opaque bytes"), "test", 100)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Capture(context.Background(), strings.NewReader("opaque bytes"), "test", 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key || first.SHA256 != second.SHA256 || first.Bytes != 12 {
		t.Fatalf("unexpected manifests: %#v %#v", first, second)
	}
	objects["/bucket/"+first.Key] = []byte("changed")
	if !errors.Is(s.Verify(context.Background(), first), ErrHashMismatch) {
		t.Fatal("stored corruption was accepted")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestRejectedCaptureNeverUploads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("rejected archive reached object store") }))
	defer server.Close()
	s := New(server.URL, "bucket", "us-east-1", "fixture", "fixture")
	if _, err := s.Capture(context.Background(), strings.NewReader("too long"), "test", 2); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size error = %v", err)
	}
	if _, err := s.Capture(context.Background(), brokenReader{}, "test", 100); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("interruption error = %v", err)
	}
}
