package httptest_test

import (
	"io"
	"net/http"
	"strings"
)

func boolPtr(b bool) *bool { return &b }

func makeResponse(body string) *http.Response {
	return &http.Response{
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
