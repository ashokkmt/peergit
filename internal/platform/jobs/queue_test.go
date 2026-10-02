package jobs

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInvalidClaimTimingIsRejectedBeforeDatabaseWork(t *testing.T) {
	q := Queue{}
	for _, tt := range []struct {
		worker string
		lease  time.Duration
	}{{"", time.Second}, {"worker", 0}, {"worker", -time.Second}} {
		if _, err := q.Claim(context.Background(), tt.worker, tt.lease); err == nil {
			t.Fatal("invalid lease accepted")
		}
	}
	if err := q.Heartbeat(context.Background(), Claim{}, 0); err == nil {
		t.Fatal("invalid heartbeat accepted")
	}
	if err := q.Retry(context.Background(), Claim{}, -time.Second); err == nil {
		t.Fatal("negative retry delay accepted")
	}
}

func TestRetryErrorCodeIsSafeForPersistence(t *testing.T) {
	for _, code := range []string{"handler_failed", "deadline.expired-1"} {
		if !safeCode(code) {
			t.Errorf("safe error code rejected: %q", code)
		}
	}
	for _, code := range []string{"", "Database password leaked", strings.Repeat("a", 101), "handler/failed"} {
		if safeCode(code) {
			t.Errorf("unsafe error code accepted: %q", code)
		}
	}
}
