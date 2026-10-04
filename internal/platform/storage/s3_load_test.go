package storage

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

const phase4MaxArchiveBytes int64 = 100 << 20

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// TestLocalS3TenMaximumCaptures is an opt-in local capacity proof. It requires
// the disposable S3-compatible Compose service and leaves no test objects.
func TestLocalS3TenMaximumCaptures(t *testing.T) {
	endpoint, bucket := os.Getenv("PEERGIT_TEST_S3_ENDPOINT"), os.Getenv("PEERGIT_TEST_S3_BUCKET")
	access, secret := os.Getenv("PEERGIT_TEST_S3_ACCESS_KEY"), os.Getenv("PEERGIT_TEST_S3_SECRET_KEY")
	if endpoint == "" || bucket == "" || access == "" || secret == "" {
		t.Skip("set PEERGIT_TEST_S3_* variables to run the ten-capture local object-storage proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	store := New(endpoint, bucket, "us-east-1", access, secret)
	receipts := make([]Receipt, 10)
	errs := make([]error, len(receipts))
	var workers sync.WaitGroup
	workers.Add(len(receipts))
	started := time.Now()
	for i := range receipts {
		go func(index int) {
			defer workers.Done()
			receipt, err := store.Capture(ctx, io.LimitReader(zeroReader{}, phase4MaxArchiveBytes), "phase4-loadtest", phase4MaxArchiveBytes)
			receipts[index], errs[index] = receipt, err
		}(i)
	}
	workers.Wait()
	defer func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		for _, receipt := range receipts {
			if receipt.Key != "" {
				if err := store.Delete(cleanupCtx, receipt.Key); err != nil {
					t.Errorf("remove temporary load-test object: %v", err)
				}
			}
		}
	}()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("capture %d failed: %v", i+1, err)
		}
		if receipts[i].Bytes != phase4MaxArchiveBytes {
			t.Fatalf("capture %d stored %d bytes, want %d", i+1, receipts[i].Bytes, phase4MaxArchiveBytes)
		}
	}
	t.Logf("ten concurrent 100 MiB captures and stored-byte verification completed in %s; local measurement only", time.Since(started).Round(time.Millisecond))
}
