package request

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"peergit/internal/platform/errormanager"
)

type validInput struct {
	Name string `json:"name"`
}

func (v *validInput) Validate() error {
	if v.Name == "" {
		return errTestValidation
	}
	return nil
}

var errTestValidation = validationError("name is required")

type validationError string

func (e validationError) Error() string { return string(e) }

func TestDecodeAcceptsOneValidJSONDocument(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Ada"}  `))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	got, err := Decode[validInput](r)
	if err != nil || got.Name != "Ada" {
		t.Fatalf("Decode() = %#v, %v; want Ada, nil", got, err)
	}
}

func TestDecodeRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name, body, contentType, wantCode string
	}{
		{"empty", "", "application/json", "body_required"},
		{"null", " null \n", "application/json", "body_required"},
		{"malformed", `{"name":`, "application/json", "body_invalid_json"},
		{"trailing value", `{"name":"Ada"} {}`, "application/json", "body_trailing_data"},
		{"wrong content type", `{"name":"Ada"}`, "text/plain", "content_type_unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.contentType)
			_, err := Decode[validInput](r)
			assertErrorCode(t, err, tt.wantCode)
		})
	}
}

func TestDecodePointerDTO(t *testing.T) {
	for _, body := range []string{"null", " \n null \t", `{"name":"Ada"}`, `{"name":""}`} {
		t.Run(body, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			got, err := Decode[*validInput](r)
			switch {
			case strings.TrimSpace(body) == "null":
				assertErrorCode(t, err, "body_required")
			case body == `{"name":""}`:
				assertErrorCode(t, err, "validation_failed")
			default:
				if err != nil || got == nil || got.Name != "Ada" {
					t.Fatalf("Decode() = %#v, %v; want Ada, nil", got, err)
				}
			}
		})
	}
}

func TestDecodeRejectsOversizedBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", MaxJSONBodyBytes+1)))
	r.Header.Set("Content-Type", "application/json")
	_, err := Decode[validInput](r)
	assertErrorCode(t, err, "body_too_large")
}

func TestDecodeRunsPointerReceiverValidator(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":""}`))
	r.Header.Set("Content-Type", "application/json")
	_, err := Decode[validInput](r)
	assertErrorCode(t, err, "validation_failed")
}

func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *errormanager.Error
	if !errors.As(err, &apiErr) || apiErr.Code != code {
		t.Fatalf("error = %v, want API error code %q", err, code)
	}
}
