package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalRiskProofs(t *testing.T) {
	if os.Getenv("VERIFY_INTEGRATION") != "true" {
		t.Skip("set VERIFY_INTEGRATION=true with the dedicated Docker proof stack running")
	}
	for _, args := range [][]string{{"check"}, {"queue"}, {"storage", "--target", "local", "--step", "capture"}, {"storage", "--target", "local", "--step", "peak"}} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", append([]string{"run", "./cmd/verify"}, args...)...)
			cmd.Dir = filepath.Join("..", "..")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("risk proof failed: %v\n%s", err, out)
			}
		})
	}
}
