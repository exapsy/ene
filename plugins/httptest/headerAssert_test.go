package httptest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	htt "microservice-var/cmd/e2e/plugins/httptest"
)

func TestNewTestHeaderAssert_InvalidConfig(t *testing.T) {
	// Missing name and no assertion fields yields error
	_, err := htt.NewTestHeaderAssert(map[string]any{})
	require.Error(t, err)
}

func TestHeaderAssert_Test(t *testing.T) {
	trueVal := true

	tests := []struct {
		name           string
		cfg            map[string]any
		headers        http.Header
		wantErr        bool
		wantErrMessage string
	}{
		{
			name:    "contains success",
			cfg:     map[string]any{"name": "X", "contains": "bc", "present": &trueVal},
			headers: http.Header{"X": {"abc"}},
		},
		{
			name:           "contains missing header",
			cfg:            map[string]any{"name": "X", "contains": "bc", "present": &trueVal},
			headers:        http.Header{},
			wantErr:        true,
			wantErrMessage: "header X not found",
		},
		{
			name:    "not_contains success",
			cfg:     map[string]any{"name": "X", "not_contains": "efg", "present": &trueVal},
			headers: http.Header{"X": {"abcd"}},
		},
		{
			name:           "not_contains fails",
			cfg:            map[string]any{"name": "X", "not_contains": "bc", "present": &trueVal},
			headers:        http.Header{"X": {"bcde"}},
			wantErr:        true,
			wantErrMessage: "header X contains bc",
		},
		{
			name:    "equals success",
			cfg:     map[string]any{"name": "X", "equals": "abc", "present": &trueVal},
			headers: http.Header{"X": {"abc"}},
		},
		{
			name:           "equals fails",
			cfg:            map[string]any{"name": "X", "equals": "abc", "present": &trueVal},
			headers:        http.Header{"X": {"abcd"}},
			wantErr:        true,
			wantErrMessage: "header X does not equal abc",
		},
		{
			name:    "not_equals success",
			cfg:     map[string]any{"name": "X", "not_equals": "abc", "present": &trueVal},
			headers: http.Header{"X": {"abcd"}},
		},
		{
			name:           "not_equals fails",
			cfg:            map[string]any{"name": "X", "not_equals": "abc", "present": &trueVal},
			headers:        http.Header{"X": {"abc"}},
			wantErr:        true,
			wantErrMessage: "header X equals abc",
		},
		{
			name:    "matches success",
			cfg:     map[string]any{"name": "X", "matches": "^a.*c$", "present": &trueVal},
			headers: http.Header{"X": {"abc"}},
		},
		{
			name: "matches fails",
			cfg: map[string]any{
				"name":    "X",
				"matches": "^a[0-9]c$",
				"present": &trueVal,
			},
			headers:        http.Header{"X": {"abc"}},
			wantErr:        true,
			wantErrMessage: "header X does not match ^a[0-9]c$",
		},
		{
			name:    "not_matches success",
			cfg:     map[string]any{"name": "X", "not_matches": "^a.*c$", "present": &trueVal},
			headers: http.Header{"X": {"xyz"}},
		},
		{
			name: "not_matches fails",
			cfg: map[string]any{
				"name":        "X",
				"not_matches": "^a.*c$",
				"present":     &trueVal,
			},
			headers:        http.Header{"X": {"abc"}},
			wantErr:        true,
			wantErrMessage: "header X matches ^a.*c$",
		},
		{
			name:    "present success",
			cfg:     map[string]any{"name": "X", "present": &trueVal},
			headers: http.Header{"X": {"anything"}},
		},
		{
			name:           "present fails",
			cfg:            map[string]any{"name": "X", "present": &trueVal},
			headers:        http.Header{},
			wantErr:        true,
			wantErrMessage: "header X is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion, err := htt.NewTestHeaderAssert(tt.cfg)
			require.NoError(t, err)

			resp := &http.Response{Header: tt.headers}

			err = assertion.Test(resp)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
