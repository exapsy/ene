package httptest

import (
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

type TestSuiteTest struct {
	TestName       string
	TestKind       string
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
	TestBodyAsserts   []TestBodyAssert   `yaml:"body_asserts"`
	TestHeaderAsserts []TestHeaderAssert `yaml:"header_asserts"`
	StatusCode        int                `yaml:"status_code"`
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
	path := t.GetPathWithFixtures(opts.Fixtures)
	body := t.GetBodyWithFixtures(opts.Fixtures)
	headers := t.GetHeadersWithFixtures(opts.Fixtures)
	queryParams := t.GetQueryParamsWithFixtures(opts.Fixtures)

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

	req, err := http.NewRequestWithContext(ctx, t.Request.Method, fullURL, body)
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
		return nil, fmt.Errorf("do request: %v", err)
	}

	defer r.Body.Close()

	if err := t.testResult(r, opts); err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Message:  err.Error(),
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
	target := testSuite.Target()

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
		return fmt.Errorf("expected status code %d, got %d", t.Expect.StatusCode, r.StatusCode)
	}

	for _, bodyAssert := range t.Expect.TestBodyAsserts {
		opts := &TestBodyAssertTestOptions{
			Fixtures: opts.Fixtures,
		}
		if err := bodyAssert.Test(r, opts); err != nil {
			return fmt.Errorf("body assert failed: %v", err)
		}
	}

	for _, headerAssert := range t.Expect.TestHeaderAsserts {
		opts := &TestHeaderAssertTestOptions{
			Fixtures: opts.Fixtures,
		}
		if err := headerAssert.Test(r, opts); err != nil {
			return fmt.Errorf("header assert failed: %v", err)
		}
	}

	return nil
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
