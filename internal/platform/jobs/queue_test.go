package jobs

import (
	"context"
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
