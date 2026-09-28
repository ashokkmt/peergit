package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peergit/internal/platform/storage"
)

func TestSnapshotRequiresImmutableIdentity(t *testing.T) {
	for _, tt := range []struct {
		id  int64
		sha string
	}{{0, strings.Repeat("a", 40)}, {1, "main"}, {1, ""}} {
		if _, err := Capture(context.Background(), nil, tt.id, tt.sha, strings.NewReader("opaque"), 100); err == nil {
			t.Fatal("invalid immutable identity accepted")
		}
	}
}

func TestSnapshotBecomesVerifiedOnlyAfterStoredRead(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "corrupted"}[corrupt], func(t *testing.T) {
			var data []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					data, _ = io.ReadAll(r.Body)
					return
				}
				if corrupt {
					_, _ = io.WriteString(w, "corrupt")
				} else {
					_, _ = w.Write(data)
				}
			}))
			defer server.Close()
			s := storage.New(server.URL, "fixture", "us-east-1", "key", "secret")
			snapshot, err := Capture(context.Background(), s, 42, strings.Repeat("a", 40), strings.NewReader("opaque"), 100)
			if corrupt {
				if err == nil || snapshot.State == "verified" {
					t.Fatal("corrupted source was certified")
				}
				return
			}
			if err != nil || snapshot.State != "verified" || snapshot.Commit != "git:sha1:"+strings.Repeat("a", 40) || snapshot.ExternalRepositoryID != 42 || !strings.Contains(snapshot.LFSAndSubmoduleCompleteness, "unverified") {
				t.Fatalf("unexpected snapshot: %#v, %v", snapshot, err)
			}
		})
	}
}
