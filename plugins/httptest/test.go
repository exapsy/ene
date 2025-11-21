package httptest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"gopkg.in/yaml.v3"
)

const (
	Kind e2eframe.TestSuiteTestKind = "http"
)

// StatusMismatchError represents an HTTP status code mismatch
type StatusMismatchError struct {
	Expected int
	Actual   int
}

func (e *StatusMismatchError) Error() string {
	return fmt.Sprintf("expected status code %d, got %d", e.Expected, e.Actual)
}

// PrettyString returns a color-formatted version highlighting the diff
func (e *StatusMismatchError) PrettyString(useColor bool) string {
	if !useColor {
		return e.Error()
	}

	// ANSI color codes
	dimGray := "\033[2m\033[90m"
	green := "\033[32m"
	red := "\033[31m"
	reset := "\033[0m"

	return fmt.Sprintf("%sexpected status code%s %s%d%s%s, got%s %s%d%s",
		dimGray, reset,
		green, e.Expected, reset,
		dimGray, reset,
		red, e.Actual, reset)
}

type TestSuiteTest struct {
	TestName       string
	TestKind       string
	TestTarget     string // Optional: override suite-level target
	Request        TestSuiteTestRequest
	Expect         TestSuiteTestExpect
	TargetEndpoint string
}

type TestSuiteTestRequest struct {
	Path        string            `yaml:"path"`
	Method      string            `yaml:"method"`
	Body        string            `yaml:"body"`
	Headers     map[string]string `yaml:"headers"`
	QueryParams map[string]string `yaml:"query_params"`
	Timeout     string            `yaml:"timeout"`
}

type TestSuiteTestExpect struct {
	TestBodyAsserts   TestBodyAsserts   `yaml:"body_asserts"`
	TestHeaderAsserts TestHeaderAsserts `yaml:"header_asserts"`
	StatusCode        int               `yaml:"status_code"`
}

func (t *TestSuiteTest) Name() string {
	return t.TestName
}

func (t *TestSuiteTest) Kind() string {
	return t.TestKind
}

func (t *TestSuiteTest) GetPathWithFixtures(fixtures []e2eframe.Fixture) string {
	if t.Request.Path == "" {
		return "/"
	}

	interpolationRegex := e2eframe.FixtureInterpolationRegex
	if interpolationRegex.MatchString(t.Request.Path) {
		return e2eframe.InterpolateString(interpolationRegex, t.Request.Path, fixtures)
	}

	return t.Request.Path
}

func (t *TestSuiteTest) GetHeadersWithFixtures(fixtures []e2eframe.Fixture) map[string]string {
	if len(t.Request.Headers) == 0 {
		return t.Request.Headers
	}

	interpolatedHeaders := make(map[string]string)
	interpolationRegex := e2eframe.FixtureInterpolationRegex

	for key, value := range t.Request.Headers {
		interpolatedKey := key
		interpolatedValue := value

		if interpolationRegex.MatchString(key) {
			interpolatedKey = e2eframe.InterpolateString(interpolationRegex, key, fixtures)
		}

		if interpolationRegex.MatchString(value) {
			interpolatedValue = e2eframe.InterpolateString(interpolationRegex, value, fixtures)
		}

		interpolatedHeaders[interpolatedKey] = interpolatedValue
	}

	return interpolatedHeaders
}

func (t *TestSuiteTest) GetQueryParamsWithFixtures(fixtures []e2eframe.Fixture) map[string]string {
	interpolatedQueryParams := make(map[string]string)
	interpolationRegex := e2eframe.FixtureInterpolationRegex

	for key, value := range t.Request.QueryParams {
		interpolatedKey := key
		interpolatedValue := value

		if interpolationRegex.MatchString(key) {
			interpolatedKey = e2eframe.InterpolateString(interpolationRegex, key, fixtures)
		}

		if interpolationRegex.MatchString(value) {
			interpolatedValue = e2eframe.InterpolateString(interpolationRegex, value, fixtures)
		}

		interpolatedQueryParams[interpolatedKey] = interpolatedValue
	}

	return interpolatedQueryParams
}

func (t *TestSuiteTest) GetBodyWithFixtures(fixtures []e2eframe.Fixture) io.ReadCloser {
	if t.Request.Body == "" {
		return io.NopCloser(strings.NewReader(""))
	}

	interpolationRegex := e2eframe.FixtureInterpolationRegex
	if interpolationRegex.MatchString(t.Request.Body) {
		str := e2eframe.InterpolateString(interpolationRegex, t.Request.Body, fixtures)

		return io.NopCloser(strings.NewReader(str))
	}

	return io.NopCloser(strings.NewReader(t.Request.Body))
}

func (t *TestSuiteTest) Run(
	ctx context.Context,
	opts *e2eframe.TestSuiteTestRunOptions,
) (*e2eframe.TestResult, error) {
	// Handle nil opts
	if opts == nil {
		opts = &e2eframe.TestSuiteTestRunOptions{}
	}

	path := t.GetPathWithFixtures(opts.Fixtures)
	body := t.GetBodyWithFixtures(opts.Fixtures)
	headers := t.GetHeadersWithFixtures(opts.Fixtures)
	queryParams := t.GetQueryParamsWithFixtures(opts.Fixtures)

	// Read body content for logging
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %v", err)
	}

	// Build URL with query parameters
	fullURL := fmt.Sprintf("%s%s", t.TargetEndpoint, path)
	if len(queryParams) > 0 {
		u, err := url.Parse(fullURL)
		if err != nil {
			return nil, fmt.Errorf("parse URL: %v", err)
		}
		q := u.Query()
		for key, value := range queryParams {
			q.Set(key, value)
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	// Log request details in verbose mode
	if opts.Verbose {
		fmt.Printf("\n=== HTTP Request ===\n")
		fmt.Printf("%s %s\n", t.Request.Method, fullURL)
		if len(headers) > 0 {
			fmt.Printf("Headers:\n")
			for key, value := range headers {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
		if len(bodyBytes) > 0 {
			fmt.Printf("Body:\n%s\n", string(bodyBytes))
		}
		fmt.Printf("====================\n\n")
	}

	// Create new body reader from bytes
	req, err := http.NewRequestWithContext(ctx, t.Request.Method, fullURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("new request: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	timeout, err := time.ParseDuration(t.Request.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse timeout: %v", err)
	}

	client := &http.Client{
		Timeout: timeout,
	}

	r, err := client.Do(req)
	if err != nil {
		errMsg := t.formatRequestError(fullURL, headers, bodyBytes, err)
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Message:  errMsg,
			Err:      err,
			Passed:   false,
		}, nil
	}

	defer r.Body.Close()

	// Read response body for logging and assertions
	responseBodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %v", err)
	}

	// Log response details in verbose mode
	if opts.Verbose {
		fmt.Printf("=== HTTP Response ===\n")
		fmt.Printf("Status: %d %s\n", r.StatusCode, http.StatusText(r.StatusCode))
		if len(r.Header) > 0 {
			fmt.Printf("Headers:\n")
			for key, values := range r.Header {
				for _, value := range values {
					fmt.Printf("  %s: %s\n", key, value)
				}
			}
		}
		if len(responseBodyBytes) > 0 {
			fmt.Printf("Body:\n%s\n", string(responseBodyBytes))
		}
		fmt.Printf("=====================\n\n")
	}

	// Replace response body with buffered version for assertions
	r.Body = io.NopCloser(bytes.NewReader(responseBodyBytes))

	if err := t.testResult(r, opts); err != nil {
		errMsg := t.formatTestFailureError(fullURL, headers, bodyBytes, r.StatusCode, r.Header, responseBodyBytes, err)
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Message:  errMsg,
			Err:      err,
			Passed:   false,
		}, nil
	}

	return &e2eframe.TestResult{
		TestName: t.TestName,
		Message:  "Test passed successfully",
		Passed:   true,
	}, nil
}

func (t *TestSuiteTest) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return &yaml.TypeError{Errors: []string{"expected mapping node"}}
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "name":
			if err := value.Decode(&t.TestName); err != nil {
				return err
			}
		case "kind":
			if err := value.Decode(&t.TestKind); err != nil {
				return err
			}
		case "target":
			if err := value.Decode(&t.TestTarget); err != nil {
				return err
			}
		case "request":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("expected mapping node, got %v", value.Kind)
			}

			req := &TestSuiteTestRequest{}
			if err := value.Decode(req); err != nil {
				return err
			}

			t.Request = *req
		case "expect":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("expected mapping node, got %v", value.Kind)
			}

			expect := &TestSuiteTestExpect{}
			if err := value.Decode(expect); err != nil {
				return err
			}

			t.Expect = *expect
		default:
			return &yaml.TypeError{Errors: []string{"unknown field: " + key.Value}}
		}
	}

	return nil
}

func (t *TestSuiteTest) Initialize(testSuite e2eframe.TestSuite) error {
	var target e2eframe.Unit

	// Use per-test target if specified, otherwise use suite-level target
	if t.TestTarget != "" {
		// Find the target unit in the test suite
		units := testSuite.Units()
		found := false
		for _, unit := range units {
			if unit.Name() == t.TestTarget {
				target = unit
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("target unit '%s' not found in suite", t.TestTarget)
		}
	} else {
		target = testSuite.Target()
	}

	// Target is required
	if target == nil {
		return fmt.Errorf("target unit not found")
	}

	endpoint := target.ExternalEndpoint()
	if endpoint == "" {
		return fmt.Errorf("target unit has no endpoint")
	}

	t.TargetEndpoint = endpoint
	if t.Request.Path == "" {
		t.Request.Path = "/"
	}

	if t.Request.Method == "" {
		t.Request.Method = http.MethodGet
	}

	if t.Request.Timeout == "" {
		t.Request.Timeout = "5s"
	}

	if t.Expect.StatusCode == 0 {
		t.Expect.StatusCode = http.StatusOK
	}

	return nil
}

func (t *TestSuiteTest) testResult(r *http.Response, opts *e2eframe.TestSuiteTestRunOptions) error {
	if r.StatusCode != t.Expect.StatusCode {
		return &StatusMismatchError{
			Expected: t.Expect.StatusCode,
			Actual:   r.StatusCode,
		}
	}

	// Convert map-based body asserts to list
	bodyAsserts, err := t.Expect.TestBodyAsserts.ToTestBodyAssertList()
	if err != nil {
		return fmt.Errorf("parse body asserts: %v", err)
	}

	for _, bodyAssert := range bodyAsserts {
		opts := &TestBodyAssertTestOptions{
			Fixtures: opts.Fixtures,
		}
		if err := bodyAssert.Test(r, opts); err != nil {
			return fmt.Errorf("body assert failed: %v", err)
		}
	}

	// Convert map-based header asserts to list
	headerAsserts, err := t.Expect.TestHeaderAsserts.ToTestHeaderAssertList()
	if err != nil {
		return fmt.Errorf("parse header asserts: %v", err)
	}

	for _, headerAssert := range headerAsserts {
		opts := &TestHeaderAssertTestOptions{
			Fixtures: opts.Fixtures,
		}
		if err := headerAssert.Test(r, opts); err != nil {
			return fmt.Errorf("header assert failed: %v", err)
		}
	}

	return nil
}

// formatRequestError formats an error message with full request details
func (t *TestSuiteTest) formatRequestError(
	url string,
	headers map[string]string,
	body []byte,
	err error,
) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Request failed: %v\n\n", err))
	sb.WriteString("=== Request Details ===\n")
	sb.WriteString(fmt.Sprintf("%s %s\n", t.Request.Method, url))
	if len(headers) > 0 {
		sb.WriteString("Headers:\n")
		for key, value := range headers {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}
	if len(body) > 0 {
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", string(body)))
	}
	sb.WriteString("=======================")
	return sb.String()
}

// formatTestFailureError formats an error message with full request and response details
func (t *TestSuiteTest) formatTestFailureError(
	url string,
	requestHeaders map[string]string,
	requestBody []byte,
	statusCode int,
	responseHeaders http.Header,
	responseBody []byte,
	err error,
) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%v\n\n", err))

	sb.WriteString("=== Request Details ===\n")
	sb.WriteString(fmt.Sprintf("%s %s\n", t.Request.Method, url))
	if len(requestHeaders) > 0 {
		sb.WriteString("Headers:\n")
		for key, value := range requestHeaders {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}
	if len(requestBody) > 0 {
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", string(requestBody)))
	}

	sb.WriteString("\n=== Response Details ===\n")
	sb.WriteString(fmt.Sprintf("Status: %d %s\n", statusCode, http.StatusText(statusCode)))
	if len(responseHeaders) > 0 {
		sb.WriteString("Headers:\n")
		for key, values := range responseHeaders {
			for _, value := range values {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
			}
		}
	}
	if len(responseBody) > 0 {
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", string(responseBody)))
	}
	sb.WriteString("========================")
	return sb.String()
}

func init() {
	e2eframe.RegisterTestSuiteTestUnmarshaler(
		Kind,
		func(node *yaml.Node) (e2eframe.TestSuiteTest, error) {
			test := &TestSuiteTest{}
			if err := test.UnmarshalYAML(node); err != nil {
				return nil, err
			}

			return test, nil
		},
	)
}
