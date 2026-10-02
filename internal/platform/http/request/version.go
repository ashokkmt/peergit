package request

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func IfMatch(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(value) < 4 || value[0] != '"' || value[len(value)-1] != '"' || !strings.HasPrefix(value[1:len(value)-1], "v") {
		return 0, errors.New("If-Match must contain a version ETag")
	}
	version, err := strconv.ParseInt(value[2:len(value)-1], 10, 64)
	if err != nil || version < 1 {
		return 0, errors.New("If-Match contains an invalid version")
	}
	return version, nil
}
