package cursor

import (
	"bytes"
	"testing"
)

func TestCursorIsVersionedAndSigned(t *testing.T) {
	key, payload := bytes.Repeat([]byte("k"), 32), []byte(`{"after":"v1"}`)
	token, err := Encode(key, payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(key, token)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("Decode() = %s, %v", got, err)
	}
	for _, invalid := range []string{token + "x", "v2" + token[2:], ""} {
		if _, err := Decode(key, invalid); err == nil {
			t.Fatalf("invalid cursor accepted: %q", invalid)
		}
	}
}
