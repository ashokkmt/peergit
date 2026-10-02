package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type Envelope struct {
	Data      any        `json:"data"`
	Error     *ErrorBody `json:"error"`
	RequestID string     `json:"request_id,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, data any) error {
	return write(w, status, Envelope{Data: data, RequestID: requestID(w)})
}

func OK(w http.ResponseWriter, data any) error {
	return JSON(w, http.StatusOK, data)
}

func Created(w http.ResponseWriter, data any) error {
	return JSON(w, http.StatusCreated, data)
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func Version(w http.ResponseWriter, version int64) error {
	if version < 1 {
		return errors.New("version must be positive")
	}
	w.Header().Set("ETag", `"v`+strconv.FormatInt(version, 10)+`"`)
	return nil
}

func WriteError(w http.ResponseWriter, status int, code, message string) error {
	return write(w, status, Envelope{
		Error:     &ErrorBody{Code: code, Message: message},
		RequestID: requestID(w),
	})
}

func requestID(w http.ResponseWriter) string {
	return w.Header().Get("X-Request-ID")
}

func write(w http.ResponseWriter, status int, envelope Envelope) error {
	if status < 100 || status > 599 {
		return errors.New("invalid HTTP status code")
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, err = w.Write(append(body, '\n'))
	return err
}
