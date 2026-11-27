package errormanager

import (
	"errors"
	"log/slog"
	"net/http"

	"peergit/internal/platform/http/response"
	"peergit/internal/platform/logging"
)

// Error carries a safe public response and the internal cause for diagnostics.
type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func New(status int, code, message string, cause error) *Error {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	return &Error{Status: status, Code: code, Message: message, Cause: cause}
}

func BadRequest(code, message string, cause error) *Error {
	return New(http.StatusBadRequest, code, message, cause)
}

func UnsupportedMediaType(code, message string, cause error) *Error {
	return New(http.StatusUnsupportedMediaType, code, message, cause)
}

func PayloadTooLarge(code, message string, cause error) *Error {
	return New(http.StatusRequestEntityTooLarge, code, message, cause)
}

// Validation converts ordinary validation errors to a safe client error while
// preserving errors that already carry an explicit API status and message.
func Validation(err error) error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return err
	}
	return BadRequest("validation_failed", "request validation failed", err)
}

type Manager struct {
	logger *slog.Logger
}

func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{logger: logger}
}

// Handle writes a stable public error and records diagnostics for the request log.
func (m *Manager) Handle(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = New(http.StatusInternalServerError, "internal_error", "an unexpected error occurred", err)
	}

	status, code, message := apiErr.Status, apiErr.Code, apiErr.Message
	requestID := w.Header().Get("X-Request-ID")
	state := logging.RequestLogStateFrom(r.Context())
	if apiErr.Status >= 500 {
		if state != nil {
			state.RecordFailure(logging.RequestFailure{Status: status, Code: "internal_error", Err: err})
		} else {
			m.logger.ErrorContext(r.Context(), "request failed", "request_id", requestID, "error", err)
		}
		code = "internal_error"
		message = "an unexpected error occurred"
	} else {
		if state != nil {
			state.RecordFailure(logging.RequestFailure{Status: status, Code: code})
		} else {
			m.logger.WarnContext(r.Context(), "request rejected", "request_id", requestID, "code", apiErr.Code)
		}
	}

	if committed, ok := w.(interface{ Status() int }); ok && committed.Status() != 0 {
		return
	}
	_ = response.WriteError(w, status, code, message)
}

// Report records a failure that happened after an HTTP response was committed.
func (m *Manager) Report(r *http.Request, err error) {
	if state := logging.RequestLogStateFrom(r.Context()); state != nil {
		state.RecordFailure(logging.RequestFailure{Status: http.StatusInternalServerError, Code: "internal_error", Err: err, AfterResponse: true})
		return
	}
	m.logger.ErrorContext(r.Context(), "request failed after response started", "request_id", r.Header.Get("X-Request-ID"), "error", err)
}
