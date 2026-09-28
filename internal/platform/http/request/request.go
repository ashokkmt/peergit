package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"peergit/internal/platform/errormanager"
)

const MaxJSONBodyBytes = 1 << 20

type Validator interface {
	Validate() error
}

func Decode[T any](r *http.Request) (T, error) {
	var dst T

	if r == nil || r.Body == nil {
		return dst, errormanager.BadRequest("body_required", "request body is required", nil)
	}

	defer r.Body.Close()

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return dst, errormanager.UnsupportedMediaType("content_type_unsupported", "Content-Type must be JSON", err)
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, MaxJSONBodyBytes+1))
	if err != nil {
		return dst, errormanager.BadRequest("body_unreadable", "request body could not be read", err)
	}
	if len(body) == 0 {
		return dst, errormanager.BadRequest("body_required", "request body is required", nil)
	}
	if len(body) > MaxJSONBodyBytes {
		return dst, errormanager.PayloadTooLarge("body_too_large", "request body exceeds the allowed size", nil)
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return dst, errormanager.BadRequest("body_required", "request body must not be null", nil)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))

	if err := decoder.Decode(&dst); err != nil {
		return dst, errormanager.BadRequest("body_invalid_json", "request body must contain valid JSON", err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return dst, errormanager.BadRequest("body_trailing_data", "request body must contain exactly one JSON value", err)
	}

	if validator, ok := any(dst).(Validator); ok {
		if err := validator.Validate(); err != nil {
			return dst, errormanager.Validation(err)
		}
	} else if validator, ok := any(&dst).(Validator); ok {
		if err := validator.Validate(); err != nil {
			return dst, errormanager.Validation(err)
		}
	}

	return dst, nil
}
