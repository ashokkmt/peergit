package main

import "testing"

func TestWorkerResendKeyUsesProviderOnlyOutsideLocalAndTest(t *testing.T) {
	for _, env := range []string{"development", "test"} {
		if got := workerResendKey(env, "re_test"); got != "" {
			t.Fatalf("workerResendKey(%q) = %q", env, got)
		}
	}
	for _, env := range []string{"staging", "production"} {
		if got := workerResendKey(env, "re_test"); got != "re_test" {
			t.Fatalf("workerResendKey(%q) = %q", env, got)
		}
	}
}
