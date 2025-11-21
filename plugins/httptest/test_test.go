package httptest_test

import (
	"context"
	"io"
	"net/http"
	stdhttptest "net/http/httptest"
	"testing"

	"github.com/exapsy/ene/e2eframe"
	httptestplugin "github.com/exapsy/ene/plugins/httptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// dummySuite satisfies e2eframe.TestSuite for Initialize().
type dummySuite struct{}

func (dummySuite) Name() string                    { return "ds" }
func (dummySuite) Units() []e2eframe.Unit          { return nil }
func (dummySuite) Tests() []e2eframe.TestSuiteTest { return nil }

func (dummySuite) Run(
	ctx context.Context,
	opts *e2eframe.RunTestOptions,
) ([]e2eframe.TestResult, int, error) {
	return nil, 0, nil
}
func (dummySuite) Target() e2eframe.Unit { return dummyUnit{} }

type dummyUnit struct{}

func (dummyUnit) Name() string { return "u" }
func (dummyUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	return nil
}
func (dummyUnit) WaitForReady(ctx context.Context) error                   { return nil }
func (dummyUnit) Stop() error                                              { return nil }
func (dummyUnit) ExternalEndpoint() string                                 { return "http://example.com" }
func (dummyUnit) LocalEndpoint() string                                    { return "" }
func (dummyUnit) Get(key string) (string, error)                           { return "", nil }
func (dummyUnit) GetEnvRaw(_ *e2eframe.GetEnvRawOptions) map[string]string { return nil }
func (dummyUnit) SetEnvs(env map[string]string)                            {}

func TestUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		want    httptestplugin.TestSuiteTest
		wantErr bool
	}{
		{
			name: "valid full mapping",
			yamlStr: `
name: foo
kind: http
request:
  path: /ping
  method: POST
  timeout: 2s
  headers:
    X-A: v1
  body: hello
expect:
  status_code: 201
`,
			want: httptestplugin.TestSuiteTest{
				TestName: "foo",
				TestKind: "http",
				Request: httptestplugin.TestSuiteTestRequest{
					Method:  "POST",
					Path:    "/ping",
					Body:    "hello",
					Timeout: "2s",
					Headers: map[string]string{"X-A": "v1"},
				},
				Expect: httptestplugin.TestSuiteTestExpect{
					StatusCode: 201,
				},
			},
		},
		{
			name:    "unknown field",
			yamlStr: `name: foo\nkind: http\nextra: x`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got httptestplugin.TestSuiteTest

			err := yaml.Unmarshal([]byte(tt.yamlStr), &got)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.TestName, got.TestName)
			assert.Equal(t, tt.want.TestKind, got.TestKind)
			assert.Equal(t, tt.want.Request.Path, got.Request.Path)
			assert.Equal(t, tt.want.Request.Method, got.Request.Method)
			assert.Equal(t, tt.want.Request.Timeout, got.Request.Timeout)
			assert.Equal(t, tt.want.Request.Body, got.Request.Body)
			assert.Equal(t, tt.want.Request.Headers, got.Request.Headers)
			assert.Equal(t, tt.want.Expect.StatusCode, got.Expect.StatusCode)
		})
	}
}

func TestRunAgainstServer(t *testing.T) {
	tests := []struct {
		name       string
		serverCode int
		serverBody string
		expectPass bool
		expectMsg  string
	}{
		{
			name:       "200 OK no asserts",
			serverCode: 200,
			serverBody: `{} `,
			expectPass: true,
			expectMsg:  "Test passed successfully",
		},
		{
			name:       "status mismatch",
			serverCode: 404,
			serverBody: `oops`,
			expectPass: false,
			expectMsg:  "expected status code 200, got 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// real HTTP server
			srv := stdhttptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.serverCode)
					_, _ = io.WriteString(w, tt.serverBody)
				}),
			)
			defer srv.Close()

			ts := &httptestplugin.TestSuiteTest{
				TestName:       "t1",
				TargetEndpoint: srv.URL,
				Request: httptestplugin.TestSuiteTestRequest{
					Path:    "/",
					Method:  http.MethodGet,
					Timeout: "1s",
				},
				Expect: httptestplugin.TestSuiteTestExpect{
					StatusCode: http.StatusOK,
				},
			}

			// Run
			res, err := ts.Run(context.Background(), nil)
			require.NoError(t, err)
			assert.Equal(t, "t1", res.TestName)
			assert.Equal(t, tt.expectPass, res.Passed)
			assert.Contains(t, res.Message, tt.expectMsg)
		})
	}
}

func TestRunWithVerboseMode(t *testing.T) {
	// Create a real HTTP server
	srv := stdhttptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Header", "test-value")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		}),
	)
	defer srv.Close()

	ts := &httptestplugin.TestSuiteTest{
		TestName:       "verbose-test",
		TargetEndpoint: srv.URL,
		Request: httptestplugin.TestSuiteTestRequest{
			Path:    "/test",
			Method:  http.MethodPost,
			Timeout: "1s",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"X-Test":       "header-value",
			},
			Body: `{"test":"data"}`,
		},
		Expect: httptestplugin.TestSuiteTestExpect{
			StatusCode: http.StatusOK,
		},
	}

	// Run with verbose mode enabled
	res, err := ts.Run(context.Background(), &e2eframe.TestSuiteTestRunOptions{
		Verbose: true,
	})
	require.NoError(t, err)
	assert.True(t, res.Passed)
}

func TestRunFailureIncludesRequestDetails(t *testing.T) {
	// Create a server that returns 404
	srv := stdhttptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Error-Code", "NOT_FOUND")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"resource not found"}`)
		}),
	)
	defer srv.Close()

	ts := &httptestplugin.TestSuiteTest{
		TestName:       "failure-test",
		TargetEndpoint: srv.URL,
		Request: httptestplugin.TestSuiteTestRequest{
			Path:    "/api/users/123",
			Method:  http.MethodGet,
			Timeout: "1s",
			Headers: map[string]string{
				"Authorization": "Bearer test-token",
				"Accept":        "application/json",
			},
			QueryParams: map[string]string{
				"include": "profile",
			},
		},
		Expect: httptestplugin.TestSuiteTestExpect{
			StatusCode: http.StatusOK,
		},
	}

	// Run the test - it should fail
	res, err := ts.Run(context.Background(), &e2eframe.TestSuiteTestRunOptions{
		Verbose: false,
	})
	require.NoError(t, err)
	assert.False(t, res.Passed)

	// Verify error message includes request details
	assert.Contains(t, res.Message, "GET")
	assert.Contains(t, res.Message, "/api/users/123")
	assert.Contains(t, res.Message, "Authorization: Bearer test-token")
	assert.Contains(t, res.Message, "Accept: application/json")
	assert.Contains(t, res.Message, "include=profile")

	// Verify error message includes response details
	assert.Contains(t, res.Message, "Status: 404")
	assert.Contains(t, res.Message, "X-Error-Code: NOT_FOUND")
	assert.Contains(t, res.Message, `{"error":"resource not found"}`)
	assert.Contains(t, res.Message, "=== Request Details ===")
	assert.Contains(t, res.Message, "=== Response Details ===")
}

func TestRunRequestErrorIncludesDetails(t *testing.T) {
	// Use an invalid URL that will cause request to fail
	ts := &httptestplugin.TestSuiteTest{
		TestName:       "request-error-test",
		TargetEndpoint: "http://invalid-host-that-does-not-exist-12345.local",
		Request: httptestplugin.TestSuiteTestRequest{
			Path:    "/api/test",
			Method:  http.MethodPost,
			Timeout: "1s",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"foo":"bar"}`,
		},
		Expect: httptestplugin.TestSuiteTestExpect{
			StatusCode: http.StatusOK,
		},
	}

	// Run the test - it should fail with request error
	res, err := ts.Run(context.Background(), &e2eframe.TestSuiteTestRunOptions{
		Verbose: false,
	})
	require.NoError(t, err)
	assert.False(t, res.Passed)

	// Verify error message includes request details
	assert.Contains(t, res.Message, "Request failed:")
	assert.Contains(t, res.Message, "POST")
	assert.Contains(t, res.Message, "/api/test")
	assert.Contains(t, res.Message, "Content-Type: application/json")
	assert.Contains(t, res.Message, `{"foo":"bar"}`)
	assert.Contains(t, res.Message, "=== Request Details ===")
}
