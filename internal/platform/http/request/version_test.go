package request

import (
	"net/http/httptest"
	"testing"
)

func TestIfMatchVersionETag(t *testing.T) {
	for input, want := range map[string]int64{`"v1"`: 1, `"v42"`: 42} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.Header.Set("If-Match", input)
		got, err := IfMatch(r)
		if err != nil || got != want {
			t.Fatalf("IfMatch(%q)=%d,%v", input, got, err)
		}
	}
	for _, input := range []string{"", "*", "W/\"v1\"", `"v0"`, `"vbad"`} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.Header.Set("If-Match", input)
		if _, err := IfMatch(r); err == nil {
			t.Fatalf("IfMatch accepted %q", input)
		}
	}
}
