package httptest_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"microservice-var/cmd/e2e/plugins/httptest"
)

func boolPtr(b bool) *bool { return &b }

func makeResponse(body string) *http.Response {
	return &http.Response{
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestBodyAssert(t *testing.T) {
	tests := []struct {
		name    string
		assert  httptest.TestBodyAssert
		body    string
		wantErr bool
	}{
		{"missing path", httptest.TestBodyAssert{Path: "", Equals: "x"}, `{"x":"value"}`, true},
		{
			"contains success",
			httptest.TestBodyAssert{Path: "msg", Contains: "ello"},
			`{"msg":"hello"}`,
			false,
		},
		{
			"contains failure",
			httptest.TestBodyAssert{Path: "msg", Contains: "z"},
			`{"msg":"hello"}`,
			true,
		},
		{
			"not contains success",
			httptest.TestBodyAssert{Path: "msg", NotContains: "z"},
			`{"msg":"hello"}`,
			false,
		},
		{
			"equals success",
			httptest.TestBodyAssert{Path: "val", Equals: "123"},
			`{"val":"123"}`,
			false,
		},
		{
			"not equals failure",
			httptest.TestBodyAssert{Path: "val", NotEquals: "123"},
			`{"val":"123"}`,
			true,
		},
		{
			"matches regex success",
			httptest.TestBodyAssert{Path: "val", Matches: `\d+`},
			`{"val":"789"}`,
			false,
		},
		{
			"not matches regex success",
			httptest.TestBodyAssert{Path: "val", NotMatches: `abc`},
			`{"val":"123"}`,
			false,
		},
		{
			"present true missing",
			httptest.TestBodyAssert{Path: "x", Present: boolPtr(true)},
			`{}`,
			true,
		},
		{
			"present false present",
			httptest.TestBodyAssert{Path: "x", Present: boolPtr(false)},
			`{"x":1}`,
			true,
		},
		{
			"size string success",
			httptest.TestBodyAssert{Path: "s", Size: 5},
			`{"s":"hello"}`,
			false,
		},
		{
			"size array failure",
			httptest.TestBodyAssert{Path: "arr", Size: 2},
			`{"arr":[1,2,3]}`,
			true,
		},
		{
			"greater than success",
			httptest.TestBodyAssert{Path: "num", GreaterThan: 5},
			`{"num":10}`,
			false,
		},
		{"less than failure", httptest.TestBodyAssert{Path: "num", LessThan: 5}, `{"num":6}`, true},
		{
			"type int success",
			httptest.TestBodyAssert{Path: "num", Type: httptest.BodyFieldTypeInt},
			`{"num":5}`,
			false,
		},
		{
			"type int failure",
			httptest.TestBodyAssert{Path: "num", Type: httptest.BodyFieldTypeInt},
			`{"num":5.5}`,
			true,
		},
		{
			"type float success",
			httptest.TestBodyAssert{Path: "num", Type: httptest.BodyFieldTypeFloat},
			`{"num":5.5}`,
			false,
		},
		{
			"type float failure",
			httptest.TestBodyAssert{Path: "num", Type: httptest.BodyFieldTypeFloat},
			`{"num":5}`,
			true,
		},
		{
			"type bool success",
			httptest.TestBodyAssert{Path: "b", Type: httptest.BodyFieldTypeBool},
			`{"b":true}`,
			false,
		},
		{
			"type array success",
			httptest.TestBodyAssert{Path: "a", Type: httptest.BodyFieldTypeArray},
			`{"a":[1]}`,
			false,
		},
		{
			"type object success",
			httptest.TestBodyAssert{Path: "o", Type: httptest.BodyFieldTypeObject},
			`{"o":{"k":"v"}}`,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := makeResponse(tt.body)

			err := tt.assert.Test(resp)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
