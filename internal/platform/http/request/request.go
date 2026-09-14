package request

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type Validator interface {
	Validate() error
}

func Decode[T any](r *http.Request) (T, error) {
	var dst T

	if r.Body == nil {
		return dst, errors.New("request body is required")
	}

	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&dst); err != nil {
		if errors.Is(err, io.EOF) {
			return dst, errors.New("request body is required")
		}

		return dst, errors.New("invalid JSON body")
	}

	if validator, ok := any(dst).(Validator); ok {
		if err := validator.Validate(); err != nil {
			return dst, err
		}
	}

	return dst, nil
}